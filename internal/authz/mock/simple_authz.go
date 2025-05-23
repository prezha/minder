// SPDX-FileCopyrightText: Copyright 2023 The Minder Authors
// SPDX-License-Identifier: Apache-2.0

// Package mock provides a no-op implementation of the minder the authorization client
package mock

import (
	"context"
	"slices"
	"sync/atomic"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"

	"github.com/mindersec/minder/internal/authz"
	minderv1 "github.com/mindersec/minder/pkg/api/protobuf/go/minder/v1"
)

// SimpleClient maintains a list of authorized projects, suitable for use in tests.
type SimpleClient struct {
	Allowed     []uuid.UUID
	Assignments map[uuid.UUID][]*minderv1.RoleAssignment

	// Adoptions is a map of child project to parent project
	Adoptions map[uuid.UUID]uuid.UUID

	// OrphanCalls is a counter for the number of times Orphan is called
	OrphanCalls atomic.Int32
}

var _ authz.Client = &SimpleClient{}

// Check implements authz.Client
func (n *SimpleClient) Check(_ context.Context, _ string, project uuid.UUID) error {
	if slices.Contains(n.Allowed, project) {
		return nil
	}
	return authz.ErrNotAuthorized
}

// Write implements authz.Client
func (n *SimpleClient) Write(_ context.Context, id string, role authz.Role, project uuid.UUID) error {
	n.Allowed = append(n.Allowed, project)
	if n.Assignments == nil {
		n.Assignments = make(map[uuid.UUID][]*minderv1.RoleAssignment)
	}
	n.Assignments[project] = append(n.Assignments[project], &minderv1.RoleAssignment{
		Subject: id,
		Role:    string(role),
		Project: proto.String(project.String()),
	})
	return nil
}

// Delete implements authz.Client
func (n *SimpleClient) Delete(_ context.Context, id string, role authz.Role, project uuid.UUID) error {
	index := slices.Index(n.Allowed, project)
	if index != -1 {
		n.Allowed[index] = n.Allowed[len(n.Allowed)-1]
		n.Allowed = n.Allowed[:len(n.Allowed)-1]
	}
	n.Assignments[project] = slices.DeleteFunc(n.Assignments[project], func(a *minderv1.RoleAssignment) bool {
		return a.Subject == id && a.Role == string(role)
	})
	return nil
}

// DeleteUser implements authz.Client
func (n *SimpleClient) DeleteUser(_ context.Context, user string) error {
	for p, as := range n.Assignments {
		n.Assignments[p] = slices.DeleteFunc(as, func(a *minderv1.RoleAssignment) bool {
			return a.Subject == user
		})
	}
	n.Allowed = nil
	return nil
}

// AssignmentsToProject implements authz.Client
func (n *SimpleClient) AssignmentsToProject(_ context.Context, p uuid.UUID) ([]*minderv1.RoleAssignment, error) {
	if n.Assignments == nil {
		return nil, nil
	}

	if _, ok := n.Assignments[p]; !ok {
		return nil, nil
	}

	// copy data to avoid modifying the original
	assignments := make([]*minderv1.RoleAssignment, len(n.Assignments[p]))
	for i, a := range n.Assignments[p] {
		assignments[i] = proto.Clone(a).(*minderv1.RoleAssignment)
	}

	return assignments, nil
}

// ProjectsForUser implements authz.Client
func (n *SimpleClient) ProjectsForUser(_ context.Context, _ string) ([]uuid.UUID, error) {
	return n.Allowed, nil
}

// PrepareForRun implements authz.Client
func (*SimpleClient) PrepareForRun(_ context.Context) error {
	return nil
}

// MigrateUp implements authz.Client
func (*SimpleClient) MigrateUp(_ context.Context) error {
	return nil
}

// Adopt implements authz.Client
func (n *SimpleClient) Adopt(_ context.Context, p, c uuid.UUID) error {

	if n.Adoptions == nil {
		n.Adoptions = make(map[uuid.UUID]uuid.UUID)
	}

	n.Adoptions[c] = p
	return nil
}

// Orphan implements authz.Client
func (n *SimpleClient) Orphan(_ context.Context, _, c uuid.UUID) error {
	n.OrphanCalls.Add(int32(1))
	if n.Adoptions == nil {
		return nil
	}

	delete(n.Adoptions, c)

	return nil
}
