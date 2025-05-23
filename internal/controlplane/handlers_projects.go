// SPDX-FileCopyrightText: Copyright 2024 The Minder Authors
// SPDX-License-Identifier: Apache-2.0

package controlplane

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/mindersec/minder/internal/auth"
	"github.com/mindersec/minder/internal/db"
	"github.com/mindersec/minder/internal/engine/engcontext"
	"github.com/mindersec/minder/internal/projects"
	"github.com/mindersec/minder/internal/projects/features"
	"github.com/mindersec/minder/internal/util"
	minderv1 "github.com/mindersec/minder/pkg/api/protobuf/go/minder/v1"
	"github.com/mindersec/minder/pkg/flags"
)

// ListProjects returns the list of projects for the current user
func (s *Server) ListProjects(
	ctx context.Context,
	_ *minderv1.ListProjectsRequest,
) (*minderv1.ListProjectsResponse, error) {
	id := auth.IdentityFromContext(ctx)

	// Not sure if we still need to do this at all, but we only create database users
	// for users registered in the primary ("") provider.
	if id != nil && id.String() == id.UserID {
		_, err := s.store.GetUserBySubject(ctx, id.String())
		if err != nil {
			return nil, status.Errorf(codes.Internal, "error getting user: %v", err)
		}
	}

	projs, err := s.authzClient.ProjectsForUser(ctx, id.String())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "error getting projects for user: %v", err)
	}

	resp := minderv1.ListProjectsResponse{}

	for _, projectID := range projs {
		project, err := s.store.GetProjectByID(ctx, projectID)
		if err != nil {
			// project was deleted while we were iterating
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			return nil, status.Errorf(codes.Internal, "error getting project: %v", err)
		}

		var description, displayName string
		meta, err := projects.ParseMetadata(&project)
		// ignore error if we can't parse the metadata. This information is not critical... yet.
		if err != nil {
			log.Ctx(ctx).Error().Err(err).Msg("failed to parse metadata")
			description = ""
			displayName = project.Name
		} else {
			description = meta.Public.Description
			displayName = meta.Public.DisplayName
		}

		resp.Projects = append(resp.Projects, &minderv1.Project{
			ProjectId:   project.ID.String(),
			Name:        project.Name,
			Description: description,
			DisplayName: displayName,
			CreatedAt:   timestamppb.New(project.CreatedAt),
			UpdatedAt:   timestamppb.New(project.UpdatedAt),
		})
	}
	return &resp, nil
}

// ListChildProjects returns the list of subprojects for the current project
func (s *Server) ListChildProjects(
	ctx context.Context,
	req *minderv1.ListChildProjectsRequest,
) (*minderv1.ListChildProjectsResponse, error) {
	entityCtx := engcontext.EntityFromContext(ctx)
	projectID := entityCtx.Project.ID

	var projs []*minderv1.Project
	var err error

	if req.Recursive {
		projs, err = s.getChildProjects(ctx, projectID)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "error getting subprojects: %v", err)
		}
	} else {
		projs, err = s.getImmediateChildrenProjects(ctx, projectID)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "error getting subprojects: %v", err)
		}
	}

	resp := minderv1.ListChildProjectsResponse{
		Projects: projs,
	}
	return &resp, nil
}

func (s *Server) getChildProjects(ctx context.Context, projectID uuid.UUID) ([]*minderv1.Project, error) {
	projs, err := s.store.GetChildrenProjects(ctx, projectID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "error getting subprojects: %v", err)
	}

	out := make([]*minderv1.Project, 0, len(projs))
	for _, project := range projs {
		out = append(out, &minderv1.Project{
			ProjectId:   project.ID.String(),
			Name:        project.Name,
			Description: "",
			// TODO: We need to agree on how to handle metadata for subprojects
			DisplayName: project.Name,
			CreatedAt:   timestamppb.New(project.CreatedAt),
			UpdatedAt:   timestamppb.New(project.UpdatedAt),
		})
	}

	return out, nil
}

func (s *Server) getImmediateChildrenProjects(ctx context.Context, projectID uuid.UUID) ([]*minderv1.Project, error) {
	projs, err := s.store.GetImmediateChildrenProjects(ctx, projectID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "error getting subprojects: %v", err)
	}

	out := make([]*minderv1.Project, 0, len(projs))
	for _, project := range projs {
		out = append(out, &minderv1.Project{
			ProjectId:   project.ID.String(),
			Name:        project.Name,
			Description: "",
			// TODO: We need to agree on how to handle metadata for subprojects
			DisplayName: project.Name,
			CreatedAt:   timestamppb.New(project.CreatedAt),
			UpdatedAt:   timestamppb.New(project.UpdatedAt),
		})
	}

	return out, nil
}

// CreateProject either create a new top-level Project (if req.Context.Project is nil/empty),
// or creates a sub-project if permitted (if req.Context.Project is set).
// The project name must be unique within the parent project, or across all projects if
// creating a top-level project.
//
// Because this may be called in a non-project context (to create a top-level project),
// we need to _explicitly_ perform project authorization checks if project is non-nil/empty.
func (s *Server) CreateProject(
	ctx context.Context,
	req *minderv1.CreateProjectRequest,
) (*minderv1.CreateProjectResponse, error) {
	parentProjectID, err := getProjectIDFromRequest(ctx, req, s.store)
	if err != nil && !errors.Is(err, ErrNoProjectInContext) {
		return nil, err
	}

	var project *db.Project
	if parentProjectID != uuid.Nil {
		// Verify permissions if we have a parent
		relationName := relationAsName(minderv1.Relation_RELATION_CREATE)
		if err := s.authzClient.Check(ctx, relationName, parentProjectID); err != nil {
			return nil, util.UserVisibleError(
				codes.PermissionDenied, "user %q is not authorized to perform this operation on project %q",
				auth.IdentityFromContext(ctx).Human(), parentProjectID)
		}

		if !features.ProjectAllowsProjectHierarchyOperations(ctx, s.store, parentProjectID) {
			return nil, util.UserVisibleError(codes.PermissionDenied,
				"project does not allow project hierarchy operations")
		}
		tx, err := s.store.BeginTransaction()
		if err != nil {
			return nil, status.Errorf(codes.Internal, "error starting transaction: %v", err)
		}
		defer s.store.Rollback(tx)
		qtx := s.store.GetQuerierWithTransaction(tx)

		project, err = s.projectCreator.ProvisionChildProject(ctx, qtx, parentProjectID, req.Name)
		if err != nil {
			return nil, err
		}

		if err := s.store.Commit(tx); err != nil {
			return nil, status.Errorf(codes.Internal, "error committing transaction: %v", err)
		}

	} else {
		// This is a top-level project creation request.
		// We need to check if the user has the right to create projects in the system.
		if !flags.Bool(ctx, s.featureFlags, flags.ProjectCreateDelete) {
			return nil, util.UserVisibleError(codes.Unimplemented, "cannot create a new top-level project")
		}

		id := auth.IdentityFromContext(ctx)
		if id.String() == "" {
			return nil, util.UserVisibleError(codes.Unauthenticated, "cannot determine user ID")
		}

		tx, err := s.store.BeginTransaction()
		if err != nil {
			return nil, status.Errorf(codes.Internal, "error starting transaction: %v", err)
		}
		defer s.store.Rollback(tx)
		qtx := s.store.GetQuerierWithTransaction(tx)
		project, err = s.projectCreator.ProvisionSelfEnrolledProject(ctx, qtx, req.Name, id.String())
		if err != nil {
			return nil, err
		}

		if err := s.store.Commit(tx); err != nil {
			return nil, status.Errorf(codes.Internal, "error committing transaction: %v", err)
		}
	}

	if project == nil {
		return nil, status.Errorf(codes.Internal, "project is nil after creation")
	}

	return &minderv1.CreateProjectResponse{
		Project: &minderv1.Project{
			ProjectId:   project.ID.String(),
			Name:        project.Name,
			Description: "",
			CreatedAt:   timestamppb.New(project.CreatedAt),
			UpdatedAt:   timestamppb.New(project.UpdatedAt),
		},
	}, nil
}

// DeleteProject deletes a project or subproject.
func (s *Server) DeleteProject(
	ctx context.Context,
	_ *minderv1.DeleteProjectRequest,
) (*minderv1.DeleteProjectResponse, error) {
	entityCtx := engcontext.EntityFromContext(ctx)
	projectID := entityCtx.Project.ID

	tx, err := s.store.BeginTransaction()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "error starting transaction: %v", err)
	}
	defer s.store.Rollback(tx)

	qtx := s.store.GetQuerierWithTransaction(tx)

	subProject, err := qtx.GetProjectByID(ctx, projectID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, status.Errorf(codes.NotFound, "project not found")
		}
		return nil, status.Errorf(codes.Internal, "error getting project: %v", err)
	}

	if subProject.ParentID.Valid {
		// The parent is supposed to have the feature flag, not the subproject
		if !features.ProjectAllowsProjectHierarchyOperations(ctx, s.store, subProject.ParentID.UUID) {
			return nil, util.UserVisibleError(codes.PermissionDenied,
				"project does not allow project hierarchy operations")
		}
	} else {
		if !flags.Bool(ctx, s.featureFlags, flags.ProjectCreateDelete) {
			return nil, util.UserVisibleError(codes.InvalidArgument, "cannot delete a top-level project")
		}
	}

	if err := s.projectDeleter.DeleteProject(ctx, projectID, qtx); err != nil {
		return nil, status.Errorf(codes.Internal, "error deleting project: %v", err)
	}

	if err := s.store.Commit(tx); err != nil {
		return nil, status.Errorf(codes.Internal, "error committing transaction: %v", err)
	}

	return &minderv1.DeleteProjectResponse{
		ProjectId: projectID.String(),
	}, nil
}

// UpdateProject updates a project. Note that this does not reparent nor
// touches the project's metadata directly. There is only a subset of
// fields that can be updated.
func (s *Server) UpdateProject(
	ctx context.Context,
	req *minderv1.UpdateProjectRequest,
) (*minderv1.UpdateProjectResponse, error) {
	entityCtx := engcontext.EntityFromContext(ctx)
	projectID := entityCtx.Project.ID

	tx, err := s.store.BeginTransaction()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "error starting transaction: %v", err)
	}
	defer s.store.Rollback(tx)

	qtx := s.store.GetQuerierWithTransaction(tx)

	project, err := qtx.GetProjectByID(ctx, projectID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, util.UserVisibleError(codes.NotFound, "project not found")
		}
		return nil, status.Errorf(codes.Internal, "error getting project: %v", err)
	}

	meta, err := projects.ParseMetadata(&project)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "error parsing metadata: %v", err)
	}

	if req.GetDisplayName() != "" {
		meta.Public.DisplayName = req.GetDisplayName()
	} else {
		// Display name cannot be empty, it will
		// default to the project name.
		meta.Public.DisplayName = project.Name
	}

	meta.Public.Description = req.GetDescription()

	serialized, err := projects.SerializeMetadata(meta)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "error serializing metadata: %v", err)
	}

	outproj, err := qtx.UpdateProjectMeta(ctx, db.UpdateProjectMetaParams{
		ID:       project.ID,
		Metadata: serialized,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "error updating project: %v", err)
	}

	if err := s.store.Commit(tx); err != nil {
		return nil, status.Errorf(codes.Internal, "error committing transaction: %v", err)
	}

	return &minderv1.UpdateProjectResponse{
		Project: &minderv1.Project{
			ProjectId:   outproj.ID.String(),
			Name:        outproj.Name,
			Description: meta.Public.Description,
			DisplayName: meta.Public.DisplayName,
			CreatedAt:   timestamppb.New(outproj.CreatedAt),
			UpdatedAt:   timestamppb.New(outproj.UpdatedAt),
		},
	}, nil
}

// PatchProject patches a project. Note that this does not reparent nor
// touches the project's metadata directly. There is only a subset of
// fields that can be updated.
func (s *Server) PatchProject(
	ctx context.Context,
	req *minderv1.PatchProjectRequest,
) (*minderv1.PatchProjectResponse, error) {
	entityCtx := engcontext.EntityFromContext(ctx)
	projectID := entityCtx.Project.ID

	tx, err := s.store.BeginTransaction()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "error starting transaction: %v", err)
	}
	defer s.store.Rollback(tx)

	qtx := s.store.GetQuerierWithTransaction(tx)

	project, err := qtx.GetProjectByID(ctx, projectID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, util.UserVisibleError(codes.NotFound, "project not found")
		}
		return nil, status.Errorf(codes.Internal, "error getting project: %v", err)
	}

	meta, err := projects.ParseMetadata(&project)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "error parsing metadata: %v", err)
	}

	req.GetUpdateMask().Normalize()
	for _, path := range req.GetUpdateMask().GetPaths() {
		switch path {
		case "display_name":
			meta.Public.DisplayName = req.GetPatch().GetDisplayName()
		case "description":
			meta.Public.Description = req.GetPatch().GetDescription()
		}
	}

	serialized, err := projects.SerializeMetadata(meta)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "error serializing metadata: %v", err)
	}

	outproj, err := qtx.UpdateProjectMeta(ctx, db.UpdateProjectMetaParams{
		ID:       project.ID,
		Metadata: serialized,
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "error updating project: %v", err)
	}

	if err := s.store.Commit(tx); err != nil {

		return nil, status.Errorf(codes.Internal, "error committing transaction: %v", err)
	}

	return &minderv1.PatchProjectResponse{
		Project: &minderv1.Project{
			ProjectId:   outproj.ID.String(),
			Name:        outproj.Name,
			Description: meta.Public.Description,
			DisplayName: meta.Public.DisplayName,
			CreatedAt:   timestamppb.New(outproj.CreatedAt),
			UpdatedAt:   timestamppb.New(outproj.UpdatedAt),
		},
	}, nil
}
