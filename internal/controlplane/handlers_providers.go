// SPDX-FileCopyrightText: Copyright 2024 The Minder Authors
// SPDX-License-Identifier: Apache-2.0

package controlplane

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/mindersec/minder/internal/crypto"
	"github.com/mindersec/minder/internal/db"
	"github.com/mindersec/minder/internal/engine/engcontext"
	"github.com/mindersec/minder/internal/providers"
	"github.com/mindersec/minder/internal/util"
	cursorutil "github.com/mindersec/minder/internal/util/cursor"
	minderv1 "github.com/mindersec/minder/pkg/api/protobuf/go/minder/v1"
	config "github.com/mindersec/minder/pkg/config/server"
)

// CreateProvider implements the CreateProvider RPC method.
func (s *Server) CreateProvider(
	ctx context.Context, req *minderv1.CreateProviderRequest,
) (*minderv1.CreateProviderResponse, error) {
	entityCtx := engcontext.EntityFromContext(ctx)
	projectID := entityCtx.Project.ID
	provider := req.GetProvider()

	if provider == nil {
		return nil, status.Errorf(codes.InvalidArgument, "provider is required")
	}

	if provider.GetVersion() == "" {
		provider.Version = "v1"
	}

	var provConfig json.RawMessage
	if provider.Config != nil {
		var marshallErr error

		provConfig, marshallErr = provider.Config.MarshalJSON()
		if marshallErr != nil {
			return nil, status.Errorf(codes.InvalidArgument, "error marshalling provider provConfig: %v", marshallErr)
		}
	} else {
		provConfig = json.RawMessage([]byte("{}"))
		zerolog.Ctx(ctx).Debug().Msg("no provider provConfig, will use default")
	}

	var configErr providers.ErrProviderInvalidConfig
	dbProv, err := s.providerManager.CreateFromConfig(
		ctx, db.ProviderClass(provider.GetClass()), projectID, provider.Name, provConfig)
	if db.ErrIsUniqueViolation(err) {
		zerolog.Ctx(ctx).Error().Err(err).Msg("provider already exists")
		return nil, util.UserVisibleError(codes.AlreadyExists, "provider already exists")
	} else if errors.As(err, &configErr) {
		zerolog.Ctx(ctx).Error().Err(err).Msg("provider config does not validate")
		return nil, util.UserVisibleError(codes.InvalidArgument, "invalid provider config: %s", configErr.Details)
	} else if err != nil {
		return nil, status.Errorf(codes.Internal, "error creating provider: %v", err)
	}

	prov, err := protobufProviderFromDB(ctx, s.store, s.cryptoEngine, &s.cfg.Provider, dbProv)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "error converting provider to protobuf: %v", err)
	}

	return &minderv1.CreateProviderResponse{
		Provider: prov,
	}, nil
}

// GetProvider gets a given provider available in a specific project.
func (s *Server) GetProvider(ctx context.Context, req *minderv1.GetProviderRequest) (*minderv1.GetProviderResponse, error) {
	entityCtx := engcontext.EntityFromContext(ctx)
	projectID := entityCtx.Project.ID

	dbProv, err := s.providerStore.GetByNameInSpecificProject(ctx, projectID, req.GetName())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, util.UserVisibleError(codes.NotFound, "provider not found")
		}
		return nil, status.Errorf(codes.Internal, "error getting provider: %v", err)
	}

	prov, err := protobufProviderFromDB(ctx, s.store, s.cryptoEngine, &s.cfg.Provider, dbProv)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "error creating provider: %v", err)
	}

	return &minderv1.GetProviderResponse{
		Provider: prov,
	}, nil
}

// ListProviders lists the providers available in a specific project.
func (s *Server) ListProviders(ctx context.Context, req *minderv1.ListProvidersRequest) (*minderv1.ListProvidersResponse, error) {
	entityCtx := engcontext.EntityFromContext(ctx)
	projectID := entityCtx.Project.ID

	params := db.ListProvidersByProjectIDPaginatedParams{
		ProjectID: projectID,
	}

	if req.Cursor != "" {
		cursor, err := cursorutil.NewProviderCursor(req.Cursor)
		if err != nil {
			return nil, err
		}

		params.CreatedAt = sql.NullTime{
			Valid: true,
			Time:  cursor.CreatedAt,
		}
	}

	if req.Limit == 0 {
		params.Limit = 10
	} else {
		params.Limit = req.Limit
	}

	list, err := s.store.ListProvidersByProjectIDPaginated(ctx, params)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return &minderv1.ListProvidersResponse{
				Providers: []*minderv1.Provider{},
			}, nil
		}
		return nil, err
	}

	zerolog.Ctx(ctx).Debug().Int("count", len(list)).Msg("providers")

	provs := make([]*minderv1.Provider, 0, len(list))
	for _, dbProv := range list {
		prov, err := protobufProviderFromDB(ctx, s.store, s.cryptoEngine, &s.cfg.Provider, &dbProv)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "error creating provider: %v", err)
		}

		provs = append(provs, prov)
	}

	cursor := ""
	if len(list) > 0 {
		c := cursorutil.ProviderCursor{
			CreatedAt: list[len(list)-1].CreatedAt,
		}
		cursor = c.String()
	}

	return &minderv1.ListProvidersResponse{
		Providers: provs,
		Cursor:    cursor,
	}, nil
}

// ListProviderClasses lists the provider classes available in the system.
func (*Server) ListProviderClasses(
	_ context.Context, _ *minderv1.ListProviderClassesRequest,
) (*minderv1.ListProviderClassesResponse, error) {
	// Note: New provider classes should be added to the providers package.
	classes := providers.ListProviderClasses()
	return &minderv1.ListProviderClassesResponse{
		ProviderClasses: classes,
	}, nil
}

// DeleteProvider deletes a provider by name from a specific project.
func (s *Server) DeleteProvider(
	ctx context.Context,
	_ *minderv1.DeleteProviderRequest,
) (*minderv1.DeleteProviderResponse, error) {
	entityCtx := engcontext.EntityFromContext(ctx)
	projectID := entityCtx.Project.ID
	providerName := entityCtx.Provider.Name

	if providerName == "" {
		return nil, status.Errorf(codes.InvalidArgument, "provider name is required")
	}

	err := s.providerManager.DeleteByName(ctx, providerName, projectID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, util.UserVisibleError(codes.NotFound, "provider not found")
		}
		return nil, status.Errorf(codes.Internal, "error deleting provider: %v", err)
	}

	return &minderv1.DeleteProviderResponse{
		Name: providerName,
	}, nil
}

// DeleteProviderByID deletes a provider by ID from a specific project.
func (s *Server) DeleteProviderByID(
	ctx context.Context,
	in *minderv1.DeleteProviderByIDRequest,
) (*minderv1.DeleteProviderByIDResponse, error) {
	entityCtx := engcontext.EntityFromContext(ctx)
	projectID := entityCtx.Project.ID

	parsedProviderID, err := uuid.Parse(in.Id)
	if err != nil {
		return nil, util.UserVisibleError(codes.InvalidArgument, "invalid provider ID")
	}

	err = s.providerManager.DeleteByID(ctx, parsedProviderID, projectID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, util.UserVisibleError(codes.NotFound, "provider not found")
		}
		return nil, status.Errorf(codes.Internal, "error deleting provider: %v", err)
	}

	return &minderv1.DeleteProviderByIDResponse{
		Id: in.Id,
	}, nil
}

// PatchProvider patches a provider by name from a specific project.
func (s *Server) PatchProvider(
	ctx context.Context,
	req *minderv1.PatchProviderRequest,
) (*minderv1.PatchProviderResponse, error) {
	entityCtx := engcontext.EntityFromContext(ctx)
	projectID := entityCtx.Project.ID
	providerName := entityCtx.Provider.Name

	if providerName == "" {
		return nil, status.Errorf(codes.InvalidArgument, "provider name is required")
	}

	if req.GetPatch() != nil && req.GetPatch().GetVersion() == "" {
		req.Patch.Version = "v1"
	}

	err := s.providerManager.PatchProviderConfig(ctx, providerName, projectID, req.GetPatch().GetConfig().AsMap())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, util.UserVisibleError(codes.NotFound, "provider not found")
		}
		return nil, status.Errorf(codes.Internal, "error patching provider: %v", err)
	}

	dbProv, err := s.providerStore.GetByNameInSpecificProject(ctx, projectID, providerName)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, util.UserVisibleError(codes.NotFound, "provider not found")
		}
		return nil, status.Errorf(codes.Internal, "error getting provider: %v", err)
	}

	prov, err := protobufProviderFromDB(ctx, s.store, s.cryptoEngine, &s.cfg.Provider, dbProv)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "error creating provider: %v", err)
	}

	return &minderv1.PatchProviderResponse{
		Provider: prov,
	}, nil
}

func protobufProviderFromDB(
	ctx context.Context, store db.Store,
	cryptoEngine crypto.Engine, pc *config.ProviderConfig, p *db.Provider,
) (*minderv1.Provider, error) {
	var cfg *structpb.Struct

	if len(p.Definition) > 0 {
		cfg = &structpb.Struct{}
		if err := protojson.Unmarshal(p.Definition, cfg); err != nil {
			return nil, fmt.Errorf("error unmarshalling provider definition: %w", err)
		}
	}

	state, err := providers.GetCredentialStateForProvider(ctx, *p, store, cryptoEngine, pc)
	if err != nil {
		// This is non-fatal
		zerolog.Ctx(ctx).Error().Err(err).Str("provider", p.Name).Msg("error getting credential")
	}

	return &minderv1.Provider{
		Id:               p.ID.String(),
		Name:             p.Name,
		Project:          p.ProjectID.String(),
		Version:          p.Version,
		Implements:       protobufProviderImplementsFromDB(ctx, *p),
		AuthFlows:        protobufProviderAuthFlowFromDB(ctx, *p),
		Config:           cfg,
		CredentialsState: state,
		Class:            string(p.Class),
	}, nil
}

func protobufProviderImplementsFromDB(ctx context.Context, p db.Provider) []minderv1.ProviderType {
	impls := make([]minderv1.ProviderType, 0, len(p.Implements))
	for _, i := range p.Implements {
		impl, ok := providers.DBToPBType(i)
		if !ok {
			zerolog.Ctx(ctx).Error().Str("type", string(i)).Str("id", p.ID.String()).Msg("unknown provider type")
			// we won't return an error here, we'll just skip the provider implementation listing
			continue
		}
		impls = append(impls, impl)
	}

	return impls
}

func protobufProviderAuthFlowFromDB(ctx context.Context, p db.Provider) []minderv1.AuthorizationFlow {
	flows := make([]minderv1.AuthorizationFlow, 0, len(p.AuthFlows))
	for _, a := range p.AuthFlows {
		flow, ok := providers.DBToPBAuthFlow(a)
		if !ok {
			zerolog.Ctx(ctx).Error().Str("flow", string(a)).Msg("unknown authorization flow")
			continue
		}
		flows = append(flows, flow)
	}

	return flows
}
