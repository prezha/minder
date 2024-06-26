// Copyright 2023 Stacklok, Inc
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package controlplane

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/stacklok/minder/internal/auth"
	"github.com/stacklok/minder/internal/authz"
	"github.com/stacklok/minder/internal/db"
	"github.com/stacklok/minder/internal/email"
	"github.com/stacklok/minder/internal/engine/engcontext"
	"github.com/stacklok/minder/internal/flags"
	"github.com/stacklok/minder/internal/invite"
	"github.com/stacklok/minder/internal/util"
	minder "github.com/stacklok/minder/pkg/api/protobuf/go/minder/v1"
)

type rpcOptionsKey struct{}

func getRpcOptions(ctx context.Context) *minder.RpcOptions {
	// nil value default is okay here
	opts, _ := ctx.Value(rpcOptionsKey{}).(*minder.RpcOptions)
	return opts
}

// EntityContextProjectInterceptor is a server interceptor that sets up the entity context project
func EntityContextProjectInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler) (any, error) {

	opts := getRpcOptions(ctx)

	if opts.GetTargetResource() == minder.TargetResource_TARGET_RESOURCE_UNSPECIFIED {
		return nil, status.Error(codes.Internal, "cannot perform authorization, because target resource is unspecified")
	}

	if opts.GetTargetResource() != minder.TargetResource_TARGET_RESOURCE_PROJECT {
		if !opts.GetNoLog() {
			zerolog.Ctx(ctx).Info().Msgf("Bypassing setting up context")
		}
		return handler(ctx, req)
	}

	server, ok := info.Server.(*Server)
	if !ok {
		return nil, status.Errorf(codes.Internal, "error casting serrver for request handling")
	}

	ctx, err := populateEntityContext(ctx, server.store, server.authzClient, req)
	if err != nil {
		return nil, err
	}

	return handler(ctx, req)
}

// ProjectAuthorizationInterceptor is a server interceptor that checks if a user is authorized on the requested project
func ProjectAuthorizationInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler) (any, error) {

	opts := getRpcOptions(ctx)

	if opts.GetTargetResource() != minder.TargetResource_TARGET_RESOURCE_PROJECT {
		if !opts.GetNoLog() {
			zerolog.Ctx(ctx).Info().Msgf("Bypassing project authorization")
		}
		return handler(ctx, req)
	}

	relation := opts.GetRelation()

	relationValue := relation.Descriptor().Values().ByNumber(relation.Number())
	if relationValue == nil {
		return nil, status.Errorf(codes.Internal, "error reading relation value %v", relation)
	}
	extension := proto.GetExtension(relationValue.Options(), minder.E_Name)
	relationName, ok := extension.(string)
	if !ok {
		return nil, status.Errorf(codes.Internal, "error getting name for requested relation %v", relation)
	}

	entityCtx := engcontext.EntityFromContext(ctx)
	server := info.Server.(*Server)

	if err := server.authzClient.Check(ctx, relationName, entityCtx.Project.ID); err != nil {
		zerolog.Ctx(ctx).Error().Err(err).Msg("authorization check failed")
		return nil, util.UserVisibleError(
			codes.PermissionDenied, "user %q is not authorized to perform this operation on project %q",
			auth.GetUserSubjectFromContext(ctx), entityCtx.Project.ID)
	}

	return handler(ctx, req)
}

// populateEntityContext populates the project in the entity context, by looking at the proto context or
// fetching the default project
func populateEntityContext(
	ctx context.Context,
	store db.Store,
	authzClient authz.Client,
	req any,
) (context.Context, error) {
	projectID, err := getProjectIDFromContext(req)
	if err != nil {
		if errors.Is(err, ErrNoProjectInContext) {
			projectID, err = getDefaultProjectID(ctx, store, authzClient)
			if err != nil {
				return ctx, err
			}
		} else {
			return ctx, err
		}
	}

	entityCtx := &engcontext.EntityContext{
		Project: engcontext.Project{
			ID: projectID,
		},
		Provider: engcontext.Provider{
			Name: getProviderFromContext(req),
		},
	}

	return engcontext.WithEntityContext(ctx, entityCtx), nil
}

func getProjectIDFromContext(req any) (uuid.UUID, error) {
	switch req := req.(type) {
	case HasProtoContextV2Compat:
		return getProjectFromContextV2Compat(req)
	case HasProtoContextV2:
		return getProjectFromContextV2(req)
	case HasProtoContext:
		return getProjectFromContext(req)
	default:
		return uuid.Nil, status.Errorf(codes.Internal, "Error extracting context from request")
	}
}

func getProviderFromContext(req any) string {
	switch req := req.(type) {
	case HasProtoContextV2Compat:
		if req.GetContextV2().GetProvider() != "" {
			return req.GetContextV2().GetProvider()
		}
		return req.GetContext().GetProvider()
	case HasProtoContextV2:
		return req.GetContextV2().GetProvider()
	case HasProtoContext:
		return req.GetContext().GetProvider()
	default:
		return ""
	}
}

func getDefaultProjectID(
	ctx context.Context,
	store db.Store,
	authzClient authz.Client,
) (uuid.UUID, error) {
	subject := auth.GetUserSubjectFromContext(ctx)

	userInfo, err := store.GetUserBySubject(ctx, subject)
	if err != nil {
		// Note that we're revealing that the user is not registered in minder
		// since the caller has a valid token (this is checked in earlier middleware).
		// Therefore, we assume it's safe output that the user is not found.
		return uuid.UUID{}, util.UserVisibleError(codes.NotFound, "user not found")
	}
	projects, err := authzClient.ProjectsForUser(ctx, userInfo.IdentitySubject)
	if err != nil {
		return uuid.UUID{}, status.Errorf(codes.Internal, "cannot find projects for user: %v", err)
	}

	if len(projects) == 0 {
		return uuid.UUID{}, util.UserVisibleError(codes.PermissionDenied, "User has no role grants in projects")
	}

	if len(projects) != 1 {
		return uuid.UUID{}, util.UserVisibleError(codes.PermissionDenied, "Cannot determine default project. Please specify one.")
	}

	return projects[0], nil
}

// Permissions API
// ensure interface implementation
var _ minder.PermissionsServiceServer = (*Server)(nil)

// ListRoles returns the list of available roles for the minder instance
func (*Server) ListRoles(_ context.Context, _ *minder.ListRolesRequest) (*minder.ListRolesResponse, error) {
	resp := minder.ListRolesResponse{
		Roles: make([]*minder.Role, 0, len(authz.AllRoles)),
	}
	for role, desc := range authz.AllRoles {
		resp.Roles = append(resp.Roles, &minder.Role{
			Name:        role.String(),
			DisplayName: authz.AllRolesDisplayName[role],
			Description: desc,
		})
	}

	return &resp, nil
}

// ListRoleAssignments returns the list of role assignments for the given project
func (s *Server) ListRoleAssignments(
	ctx context.Context,
	_ *minder.ListRoleAssignmentsRequest,
) (*minder.ListRoleAssignmentsResponse, error) {
	invitations := make([]*minder.Invitation, 0)
	// Determine the target project.
	entityCtx := engcontext.EntityFromContext(ctx)
	targetProject := entityCtx.Project.ID

	as, err := s.authzClient.AssignmentsToProject(ctx, targetProject)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "error getting role assignments: %v", err)
	}

	// Resolve the display names for the subjects
	mapIdToDisplay := make(map[string]string, len(as))
	for i := range as {
		identity, err := s.idClient.Resolve(ctx, as[i].Subject)
		if err != nil {
			// If we can't resolve the subject, report the raw ID value
			as[i].DisplayName = as[i].Subject
			if mapIdToDisplay[as[i].Subject] == "" {
				mapIdToDisplay[as[i].Subject] = as[i].Subject
			}
			zerolog.Ctx(ctx).Error().Err(err).Msg("error resolving identity")
			continue
		}
		as[i].DisplayName = identity.Human()
		if mapIdToDisplay[as[i].Subject] == "" {
			mapIdToDisplay[as[i].Subject] = identity.Human()
		}
	}

	if flags.Bool(ctx, s.featureFlags, flags.UserManagement) {
		// Add invitations, which are only stored in the Minder DB
		invites, err := s.store.ListInvitationsForProject(ctx, targetProject)
		if err != nil {
			// return the information we can and log the error
			zerolog.Ctx(ctx).Error().Err(err).Msg("error getting invitations")
		}
		for _, i := range invites {
			invitations = append(invitations, &minder.Invitation{
				Role:           i.Role,
				Email:          i.Email,
				Project:        targetProject.String(),
				CreatedAt:      timestamppb.New(i.CreatedAt),
				ExpiresAt:      invite.GetExpireIn7Days(i.UpdatedAt),
				Expired:        invite.IsExpired(i.UpdatedAt),
				Sponsor:        i.IdentitySubject,
				SponsorDisplay: mapIdToDisplay[i.IdentitySubject],
				// Code is explicitly not returned here
			})
		}
	}

	return &minder.ListRoleAssignmentsResponse{
		RoleAssignments: as,
		Invitations:     invitations,
	}, nil
}

// AssignRole assigns a role to a user on a project.
// Note that this assumes that the request has already been authorized.
func (s *Server) AssignRole(ctx context.Context, req *minder.AssignRoleRequest) (*minder.AssignRoleResponse, error) {
	role := req.GetRoleAssignment().GetRole()
	sub := req.GetRoleAssignment().GetSubject()
	inviteeEmail := req.GetRoleAssignment().GetEmail()

	// Determine the target project.
	entityCtx := engcontext.EntityFromContext(ctx)
	targetProject := entityCtx.Project.ID

	// Ensure user is not updating their own role
	err := isUserSelfUpdating(ctx, sub, inviteeEmail)
	if err != nil {
		return nil, err
	}

	// Parse role (this also validates)
	authzRole, err := authz.ParseRole(role)
	if err != nil {
		return nil, util.UserVisibleError(codes.InvalidArgument, err.Error())
	}

	// Ensure the target project exists
	_, err = s.store.GetProjectByID(ctx, targetProject)
	if err != nil {
		// If the project is not found, return an error
		if errors.Is(err, sql.ErrNoRows) {
			return nil, util.UserVisibleError(codes.InvalidArgument, "target project with ID %s not found", targetProject)
		}
		return nil, status.Errorf(codes.Internal, "error getting project: %v", err)
	}

	// Decide if it's an invitation or a role assignment
	if sub == "" && inviteeEmail != "" {
		if flags.Bool(ctx, s.featureFlags, flags.UserManagement) {
			return s.inviteUser(ctx, targetProject, authzRole, inviteeEmail)
		}
		return nil, util.UserVisibleError(codes.Unimplemented, "user management is not enabled")
	} else if sub != "" && inviteeEmail == "" {
		// Enable one or the other.
		// This is temporary until we deprecate it completely in favor of email-based role assignments
		if !flags.Bool(ctx, s.featureFlags, flags.UserManagement) {
			return s.assignRole(ctx, targetProject, authzRole, sub)
		}
		return nil, util.UserVisibleError(codes.Unimplemented, "user management is enabled, use invites instead")
	}
	return nil, util.UserVisibleError(codes.InvalidArgument, "one of subject or email must be specified")
}

func (s *Server) inviteUser(
	ctx context.Context,
	targetProject uuid.UUID,
	role authz.Role,
	inviteeEmail string,
) (*minder.AssignRoleResponse, error) {
	var userInvite db.UserInvite
	// Get the sponsor's user information (current user)
	currentUser, err := s.store.GetUserBySubject(ctx, auth.GetUserSubjectFromContext(ctx))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get user: %s", err)
	}

	// Check if the user is already invited
	existingInvites, err := s.store.GetInvitationsByEmailAndProject(ctx, db.GetInvitationsByEmailAndProjectParams{
		Email:   inviteeEmail,
		Project: targetProject,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "error getting invitations: %v", err)
	}

	// Check if there are any existing invitations for this email
	if len(existingInvites) != 0 {
		return nil, util.UserVisibleError(
			codes.AlreadyExists,
			"invitation for this email and project already exists, use update instead",
		)
	}

	// If there are no invitations for this email, great, we should create one
	// Resolve the sponsor's identity and display name
	sponsorDisplay := currentUser.IdentitySubject
	identity, err := s.idClient.Resolve(ctx, currentUser.IdentitySubject)
	if err != nil {
		zerolog.Ctx(ctx).Error().Err(err).Msg("error resolving identity")
	} else {
		sponsorDisplay = identity.Human()
	}

	// Resolve the target project's display name
	prj, err := s.store.GetProjectByID(ctx, targetProject)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get target project: %s", err)
	}

	// Create the invitation
	userInvite, err = s.store.CreateInvitation(ctx, db.CreateInvitationParams{
		Code:    invite.GenerateCode(),
		Email:   inviteeEmail,
		Role:    role.String(),
		Project: targetProject,
		Sponsor: currentUser.ID,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "error creating invitation: %v", err)
	}

	// Publish the event for sending the invitation email
	msg, err := email.NewMessage(userInvite.Email, userInvite.Code, userInvite.Role, prj.Name, sponsorDisplay)
	if err != nil {
		return nil, fmt.Errorf("error generating UUID: %w", err)
	}
	err = s.evt.Publish(email.TopicQueueInviteEmail, msg)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "error publishing event: %v", err)
	}

	// Send the invitation response
	return &minder.AssignRoleResponse{
		// Leaving the role assignment empty as it's an invitation
		Invitation: &minder.Invitation{
			Role:           userInvite.Role,
			Email:          userInvite.Email,
			Project:        userInvite.Project.String(),
			ProjectDisplay: prj.Name,
			Code:           userInvite.Code,
			Sponsor:        currentUser.IdentitySubject,
			SponsorDisplay: sponsorDisplay,
			CreatedAt:      timestamppb.New(userInvite.CreatedAt),
			ExpiresAt:      invite.GetExpireIn7Days(userInvite.UpdatedAt),
			Expired:        invite.IsExpired(userInvite.UpdatedAt),
		},
	}, nil
}

func (s *Server) assignRole(
	ctx context.Context,
	targetPrj uuid.UUID,
	role authz.Role,
	subject string,
) (*minder.AssignRoleResponse, error) {
	var err error
	// Resolve the subject to an identity
	identity, err := s.idClient.Resolve(ctx, subject)
	if err != nil {
		zerolog.Ctx(ctx).Error().Err(err).Msg("error resolving identity")
		return nil, util.UserVisibleError(codes.NotFound, "could not find identity %q", subject)
	}

	// Verify if user exists.
	// TODO: this assumes that we store all users in the database, and that we don't
	// need to namespace identify providers.  We should revisit these assumptions.
	//
	if _, err := s.store.GetUserBySubject(ctx, identity.String()); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, util.UserVisibleError(codes.NotFound, "User not found")
		}
		return nil, status.Errorf(codes.Internal, "error getting user: %v", err)
	}

	// Check in case there's an existing role assignment for the user
	as, err := s.authzClient.AssignmentsToProject(ctx, targetPrj)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "error getting role assignments: %v", err)
	}

	for _, a := range as {
		if a.Subject == identity.String() {
			return nil, util.UserVisibleError(codes.AlreadyExists, "role assignment for this user already exists, use update instead")
		}
	}

	// Assign the role to the user
	if err := s.authzClient.Write(ctx, identity.String(), role, targetPrj); err != nil {
		return nil, status.Errorf(codes.Internal, "error writing role assignment: %v", err)
	}

	respProj := targetPrj.String()
	return &minder.AssignRoleResponse{
		RoleAssignment: &minder.RoleAssignment{
			Role:    role.String(),
			Subject: identity.Human(),
			Project: &respProj,
		},
	}, nil
}

// RemoveRole removes a role from a user on a project
// Note that this assumes that the request has already been authorized.
func (s *Server) RemoveRole(ctx context.Context, req *minder.RemoveRoleRequest) (*minder.RemoveRoleResponse, error) {
	role := req.GetRoleAssignment().GetRole()
	sub := req.GetRoleAssignment().GetSubject()
	inviteeEmail := req.GetRoleAssignment().GetEmail()
	// Determine the target project.
	entityCtx := engcontext.EntityFromContext(ctx)
	targetProject := entityCtx.Project.ID

	// Parse role (this also validates)
	authzRole, err := authz.ParseRole(role)
	if err != nil {
		return nil, util.UserVisibleError(codes.InvalidArgument, err.Error())
	}

	// Validate the subject and email - decide if it's about removing an invitation or a role assignment
	if sub == "" && inviteeEmail != "" {
		if flags.Bool(ctx, s.featureFlags, flags.UserManagement) {
			return s.removeInvite(ctx, targetProject, authzRole, inviteeEmail)
		}
		return nil, util.UserVisibleError(codes.Unimplemented, "user management is not enabled")
	} else if sub != "" && inviteeEmail == "" {
		// If there's a subject, we assume it's a role assignment
		return s.removeRole(ctx, targetProject, authzRole, sub)
	}
	return nil, util.UserVisibleError(codes.InvalidArgument, "one of subject or email must be specified")
}

func (s *Server) removeInvite(
	ctx context.Context,
	targetPrj uuid.UUID,
	role authz.Role,
	inviteeEmail string,
) (*minder.RemoveRoleResponse, error) {
	// Get all invitations for this email and project
	invitesToRemove, err := s.store.GetInvitationsByEmailAndProject(ctx, db.GetInvitationsByEmailAndProjectParams{
		Email:   inviteeEmail,
		Project: targetPrj,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "error getting invitation: %v", err)
	}

	// If there are no invitations for this email, return an error
	if len(invitesToRemove) == 0 {
		return nil, util.UserVisibleError(codes.NotFound, "no invitations found for this email and project")
	}

	// Find the invitation to remove. There should be only one invitation for the given role and email in the project.
	var inviteToRemove *db.GetInvitationsByEmailAndProjectRow
	for _, i := range invitesToRemove {
		if i.Role == role.String() {
			inviteToRemove = &i
			break
		}
	}
	// If there's no invitation to remove, return an error
	if inviteToRemove == nil {
		return nil, util.UserVisibleError(codes.NotFound, "no invitation found for this role and email in the project")
	}
	// Delete the invitation
	ret, err := s.store.DeleteInvitation(ctx, inviteToRemove.Code)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "error deleting invitation: %v", err)
	}

	// Resolve the project's display name
	prj, err := s.store.GetProjectByID(ctx, ret.Project)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get project: %s", err)
	}

	// Get the sponsor's user information (current user)
	sponsorUser, err := s.store.GetUserByID(ctx, ret.Sponsor)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get user: %s", err)
	}

	// Resolve the sponsor's identity and display name
	sponsorDisplay := sponsorUser.IdentitySubject
	identity, err := s.idClient.Resolve(ctx, sponsorUser.IdentitySubject)
	if err != nil {
		zerolog.Ctx(ctx).Error().Err(err).Msg("error resolving identity")
	} else {
		sponsorDisplay = identity.Human()
	}

	// Return the response
	return &minder.RemoveRoleResponse{
		Invitation: &minder.Invitation{
			Role:           ret.Role,
			Email:          ret.Email,
			Project:        ret.Project.String(),
			Code:           ret.Code,
			CreatedAt:      timestamppb.New(ret.CreatedAt),
			ExpiresAt:      invite.GetExpireIn7Days(ret.UpdatedAt),
			Expired:        invite.IsExpired(ret.UpdatedAt),
			Sponsor:        sponsorUser.IdentitySubject,
			SponsorDisplay: sponsorDisplay,
			ProjectDisplay: prj.Name,
		},
	}, nil
}

func (s *Server) removeRole(
	ctx context.Context,
	targetProject uuid.UUID,
	roleToRemove authz.Role,
	subject string,
) (*minder.RemoveRoleResponse, error) {
	var err error
	// Resolve the subject to an identity
	identity, err := s.idClient.Resolve(ctx, subject)
	if err != nil {
		zerolog.Ctx(ctx).Error().Err(err).Msg("error resolving identity")
		return nil, util.UserVisibleError(codes.NotFound, "could not find identity %q", subject)
	}

	// Verify if user exists
	if _, err := s.store.GetUserBySubject(ctx, identity.String()); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, util.UserVisibleError(codes.NotFound, "User not found")
		}
		return nil, status.Errorf(codes.Internal, "error getting user: %v", err)
	}

	// Validate in case there's only one admin for the project and the user is trying to remove themselves
	if roleToRemove == authz.AuthzRoleAdmin {
		// Get all role assignments for the project
		as, err := s.authzClient.AssignmentsToProject(ctx, targetProject)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "error getting role assignments: %v", err)
		}
		// Count the number of admin roles
		adminRolesCnt := 0
		for _, existing := range as {
			if existing.Role == authz.AuthzRoleAdmin.String() {
				adminRolesCnt++
			}
		}
		// If there's only one admin role, return an error
		if adminRolesCnt <= 1 {
			return nil, util.UserVisibleError(codes.FailedPrecondition, "cannot remove the last admin from the project")
		}
	}

	// Delete the role assignment
	if err := s.authzClient.Delete(ctx, identity.String(), roleToRemove, targetProject); err != nil {
		return nil, status.Errorf(codes.Internal, "error writing role assignment: %v", err)
	}
	prj := targetProject.String()
	return &minder.RemoveRoleResponse{
		RoleAssignment: &minder.RoleAssignment{
			Role:    roleToRemove.String(),
			Subject: identity.Human(),
			Project: &prj,
		},
	}, nil
}

// UpdateRole updates a role for a user on a project
func (s *Server) UpdateRole(ctx context.Context, req *minder.UpdateRoleRequest) (*minder.UpdateRoleResponse, error) {
	// For the time being, ensure only one role is updated at a time
	if len(req.GetRoles()) != 1 {
		return nil, util.UserVisibleError(codes.InvalidArgument, "only one role can be updated at a time")
	}
	role := req.GetRoles()[0]
	sub := req.GetSubject()
	inviteeEmail := req.GetEmail()

	// Determine the target project.
	entityCtx := engcontext.EntityFromContext(ctx)
	targetProject := entityCtx.Project.ID

	// Ensure user is not updating their own role
	err := isUserSelfUpdating(ctx, sub, inviteeEmail)
	if err != nil {
		return nil, err
	}

	// Parse role (this also validates)
	authzRole, err := authz.ParseRole(role)
	if err != nil {
		return nil, util.UserVisibleError(codes.InvalidArgument, err.Error())
	}

	// Validate the subject and email - decide if it's about updating an invitation or a role assignment
	if sub == "" && inviteeEmail != "" {
		if flags.Bool(ctx, s.featureFlags, flags.UserManagement) {
			return s.updateInvite(ctx, targetProject, authzRole, inviteeEmail)
		}
		return nil, util.UserVisibleError(codes.Unimplemented, "user management is not enabled")
	} else if sub != "" && inviteeEmail == "" {
		// If there's a subject, we assume it's a role assignment update
		return s.updateRole(ctx, targetProject, authzRole, sub)
	}
	return nil, util.UserVisibleError(codes.InvalidArgument, "one of subject or email must be specified")
}

func (s *Server) updateInvite(
	ctx context.Context,
	targetProject uuid.UUID,
	authzRole authz.Role,
	inviteeEmail string,
) (*minder.UpdateRoleResponse, error) {
	var userInvite db.UserInvite
	// Get the sponsor's user information (current user)
	currentUser, err := s.store.GetUserBySubject(ctx, auth.GetUserSubjectFromContext(ctx))
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get user: %s", err)
	}

	// Get all invitations for this email and project
	existingInvites, err := s.store.GetInvitationsByEmailAndProject(ctx, db.GetInvitationsByEmailAndProjectParams{
		Email:   inviteeEmail,
		Project: targetProject,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "error getting invitations: %v", err)
	}

	// Exit early if there are no or multiple existing invitations for this email and project
	if len(existingInvites) == 0 {
		return nil, util.UserVisibleError(codes.NotFound, "no invitations found for this email and project")
	} else if len(existingInvites) > 1 {
		return nil, status.Errorf(codes.Internal, "multiple invitations found for this email and project")
	}

	// Begin a transaction to ensure that the invitation is updated atomically
	tx, err := s.store.BeginTransaction()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "error starting transaction: %v", err)
	}
	defer s.store.Rollback(tx)

	// At this point, there should be exactly 1 invitation.
	// Depending on the role from the request, we can either update the role and its expiration
	// or just bump the expiration date.
	// In both cases, we can use the same query.
	userInvite, err = s.store.UpdateInvitationRole(ctx, db.UpdateInvitationRoleParams{
		Code: existingInvites[0].Code,
		Role: authzRole.String(),
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "error updating invitation: %v", err)
	}

	// Resolve the project's display name
	prj, err := s.store.GetProjectByID(ctx, userInvite.Project)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to get project: %s", err)
	}
	// Resolve the sponsor's identity and display name
	identity, err := s.idClient.Resolve(ctx, currentUser.IdentitySubject)
	if err != nil {
		zerolog.Ctx(ctx).Error().Err(err).Msg("error resolving identity")
		return nil, util.UserVisibleError(codes.NotFound, "could not find identity %q", currentUser.IdentitySubject)
	}

	// Commit the transaction to persist the changes
	if err = s.store.Commit(tx); err != nil {
		return nil, status.Errorf(codes.Internal, "error committing transaction: %v", err)
	}

	// Publish the event for sending the invitation email
	// This will happen only if the role is updated (existingInvites[0].Role != authzRole.String())
	// or the role stayed the same, but the last invite update was more than a day ago
	if existingInvites[0].Role != authzRole.String() || userInvite.UpdatedAt.Sub(existingInvites[0].UpdatedAt) > 24*time.Hour {
		msg, err := email.NewMessage(userInvite.Email, userInvite.Code, userInvite.Role, prj.Name, identity.Human())
		if err != nil {
			return nil, fmt.Errorf("error generating UUID: %w", err)
		}
		err = s.evt.Publish(email.TopicQueueInviteEmail, msg)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "error publishing event: %v", err)
		}
	}

	return &minder.UpdateRoleResponse{
		Invitations: []*minder.Invitation{
			{
				Role:           userInvite.Role,
				Email:          userInvite.Email,
				Project:        userInvite.Project.String(),
				ProjectDisplay: prj.Name,
				Code:           userInvite.Code,
				Sponsor:        identity.String(),
				SponsorDisplay: identity.Human(),
				CreatedAt:      timestamppb.New(userInvite.CreatedAt),
				ExpiresAt:      invite.GetExpireIn7Days(userInvite.UpdatedAt),
				Expired:        invite.IsExpired(userInvite.UpdatedAt),
			},
		},
	}, nil
}

func (s *Server) updateRole(
	ctx context.Context,
	targetProject uuid.UUID,
	authzRole authz.Role,
	sub string,
) (*minder.UpdateRoleResponse, error) {
	// Resolve the subject to an identity
	identity, err := s.idClient.Resolve(ctx, sub)
	if err != nil {
		zerolog.Ctx(ctx).Error().Err(err).Msg("error resolving identity")
		return nil, util.UserVisibleError(codes.NotFound, "could not find identity %q", sub)
	}

	// Verify if user exists
	if _, err := s.store.GetUserBySubject(ctx, identity.String()); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, util.UserVisibleError(codes.NotFound, "User not found")
		}
		return nil, status.Errorf(codes.Internal, "error getting user: %v", err)
	}

	// Remove the existing role assignment for the user
	as, err := s.authzClient.AssignmentsToProject(ctx, targetProject)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "error getting role assignments: %v", err)
	}

	for _, a := range as {
		if a.Subject == identity.String() {
			roleToDelete, err := authz.ParseRole(a.Role)
			if err != nil {
				return nil, util.UserVisibleError(codes.Internal, err.Error())
			}
			if err := s.authzClient.Delete(ctx, identity.String(), roleToDelete, targetProject); err != nil {
				return nil, status.Errorf(codes.Internal, "error deleting previous role assignment: %v", err)
			}
		}
	}

	// Update the role assignment for the user
	if err := s.authzClient.Write(ctx, identity.String(), authzRole, targetProject); err != nil {
		return nil, status.Errorf(codes.Internal, "error writing role assignment: %v", err)
	}

	respProj := targetProject.String()
	return &minder.UpdateRoleResponse{
		RoleAssignments: []*minder.RoleAssignment{
			{
				Role:    authzRole.String(),
				Subject: identity.Human(),
				Project: &respProj,
			},
		},
	}, nil
}

// isUserSelfUpdating is used to prevent if the user is trying to update their own role
func isUserSelfUpdating(ctx context.Context, subject, inviteeEmail string) error {
	if subject != "" {
		if auth.GetUserSubjectFromContext(ctx) == subject {
			return util.UserVisibleError(codes.InvalidArgument, "cannot update your own role")
		}
	}
	if inviteeEmail != "" {
		tokenEmail, err := auth.GetUserEmailFromContext(ctx)
		if err != nil {
			return util.UserVisibleError(codes.Internal, "error getting user email from token: %v", err)
		}
		if tokenEmail == inviteeEmail {
			return util.UserVisibleError(codes.InvalidArgument, "cannot update your own role")
		}
	}
	return nil
}
