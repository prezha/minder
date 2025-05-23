// SPDX-FileCopyrightText: Copyright 2024 The Minder Authors
// SPDX-License-Identifier: Apache-2.0

// Package service encodes the business logic for dealing with data sources.
package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/mindersec/minder/internal/datasources"
	"github.com/mindersec/minder/internal/db"
	"github.com/mindersec/minder/internal/marketplaces/namespaces"
	"github.com/mindersec/minder/internal/util"
	minderv1 "github.com/mindersec/minder/pkg/api/protobuf/go/minder/v1"
	v1datasources "github.com/mindersec/minder/pkg/datasources/v1"
)

//go:generate go run go.uber.org/mock/mockgen -package mock_$GOPACKAGE -destination=./mock/$GOFILE -source=./$GOFILE

var (
	// ErrDataSourceAlreadyExists is returned when a data source already exists
	ErrDataSourceAlreadyExists = util.UserVisibleError(codes.AlreadyExists, "data source already exists")
)

// DataSourcesService is an interface that defines the methods for the data sources service.
type DataSourcesService interface {
	// GetByName returns a data source by name.
	GetByName(ctx context.Context, name string, project uuid.UUID, opts *ReadOptions) (*minderv1.DataSource, error)

	// GetByID returns a data source by ID.
	GetByID(ctx context.Context, id uuid.UUID, project uuid.UUID, opts *ReadOptions) (*minderv1.DataSource, error)

	// List lists all data sources in the given project.
	List(ctx context.Context, project uuid.UUID, opts *ReadOptions) ([]*minderv1.DataSource, error)

	// Create creates a new data source.
	Create(
		ctx context.Context,
		projectID uuid.UUID,
		subscriptionID uuid.UUID,
		ds *minderv1.DataSource,
		opts *Options,
	) (*minderv1.DataSource, error)

	// Update updates an existing data source.
	Update(
		ctx context.Context,
		projectID uuid.UUID,
		subscriptionID uuid.UUID,
		ds *minderv1.DataSource,
		opts *Options,
	) (*minderv1.DataSource, error)

	// Upsert creates a new data source if it does not exist or updates it if it already exists.
	// This is used in the subscription logic.
	Upsert(ctx context.Context, projectID uuid.UUID, subscriptionID uuid.UUID, ds *minderv1.DataSource, opts *Options) error

	// Delete deletes a data source in the given project.
	//
	// Note that one cannot delete a data source that is in use by a rule type.
	Delete(ctx context.Context, id uuid.UUID, project uuid.UUID, opts *Options) error

	// BuildDataSourceRegistry bundles up all data sources referenced in the rule type
	// into a registry.
	BuildDataSourceRegistry(ctx context.Context, rt *minderv1.RuleType, opts *Options) (*v1datasources.DataSourceRegistry, error)
}

type dataSourceService struct {
	store db.Store

	// This is a function that will begin a transaction for the service.
	// We make this a function so that we can mock it in tests.
	txBuilder func(d *dataSourceService, opts txGetter) (serviceTX, error)
}

// NewDataSourceService creates a new data source service.
func NewDataSourceService(store db.Store) *dataSourceService {
	return &dataSourceService{
		store:     store,
		txBuilder: beginTx,
	}
}

// WithTransactionBuilder sets the transaction builder for the data source service.
//
// Note this is mostly just useful for testing.
func (d *dataSourceService) WithTransactionBuilder(txBuilder func(d *dataSourceService, opts txGetter) (serviceTX, error)) {
	d.txBuilder = txBuilder
}

// Ensure that dataSourceService implements DataSourcesService.
var _ DataSourcesService = (*dataSourceService)(nil)

func (d *dataSourceService) GetByName(
	ctx context.Context, name string, project uuid.UUID, opts *ReadOptions) (*minderv1.DataSource, error) {
	return d.getDataSourceSomehow(
		ctx, project, opts, func(ctx context.Context, tx db.ExtendQuerier, projs []uuid.UUID,
		) (db.DataSource, error) {
			return getByNameQuery(ctx, tx, projs, name)
		})
}

func (d *dataSourceService) GetByID(
	ctx context.Context, id uuid.UUID, project uuid.UUID, opts *ReadOptions) (*minderv1.DataSource, error) {
	return d.getDataSourceSomehow(
		ctx, project, opts, func(ctx context.Context, tx db.ExtendQuerier, projs []uuid.UUID,
		) (db.DataSource, error) {
			return getByIDQuery(ctx, tx, projs, id)
		})
}

func (d *dataSourceService) List(
	ctx context.Context, project uuid.UUID, opts *ReadOptions) ([]*minderv1.DataSource, error) {
	stx, err := d.txBuilder(d, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to start transaction: %w", err)
	}

	defer func() {
		if stx == nil {
			// already committed
			return
		}
		if err := stx.Rollback(); err != nil {
			zerolog.Ctx(ctx).Error().Err(err).Msg("failed to rollback transaction")
		}
	}()

	tx := stx.Q()

	projs, err := listRelevantProjects(ctx, tx, project, opts.canSearchHierarchical())
	if err != nil {
		return nil, fmt.Errorf("failed to list relevant projects: %w", err)
	}

	dss, err := tx.ListDataSources(ctx, projs)
	if err != nil {
		return nil, fmt.Errorf("failed to list data sources: %w", err)
	}

	outDS := make([]*minderv1.DataSource, len(dss))

	for i, ds := range dss {
		dsfuncs, err := tx.ListDataSourceFunctions(ctx, db.ListDataSourceFunctionsParams{
			DataSourceID: ds.ID,
			ProjectID:    ds.ProjectID,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to list data source functions: %w", err)
		}

		dsProtobuf, err := dataSourceDBToProtobuf(ds, dsfuncs)
		if err != nil {
			return nil, fmt.Errorf("failed to convert data source to protobuf: %w", err)
		}

		outDS[i] = dsProtobuf
	}

	if err := stx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}
	stx = nil // Don't try to rollback

	return outDS, nil
}

// Create creates a new data source.
//
// Create handles data source creation by using a transaction to ensure atomicity.
// We first validate the data source name uniqueness, then create the data source record.
// Finally, we create function records based on the driver type.
func (d *dataSourceService) Create(
	ctx context.Context,
	projectID uuid.UUID,
	subscriptionID uuid.UUID,
	ds *minderv1.DataSource,
	opts *Options,
) (*minderv1.DataSource, error) {
	if err := ds.Validate(); err != nil {
		return nil, fmt.Errorf("data source validation failed: %w", err)
	}

	if err := namespaces.ValidateNamespacedNameRules(ds.GetName(), subscriptionID); err != nil {
		return nil, fmt.Errorf("data source validation failed: %w", err)
	}

	stx, err := d.txBuilder(d, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to start transaction: %w", err)
	}

	defer func() {
		if stx == nil {
			// already committed
			return
		}
		if err := stx.Rollback(); err != nil {
			zerolog.Ctx(ctx).Error().Err(err).Msg("failed to rollback transaction")
		}
	}()

	tx := stx.Q()

	// Check if such data source already exists in project hierarchy
	projs, err := listRelevantProjects(ctx, tx, projectID, true)
	if err != nil {
		return nil, fmt.Errorf("failed to list relevant projects: %w", err)
	}
	existing, err := tx.GetDataSourceByName(ctx, db.GetDataSourceByNameParams{
		Name:     ds.GetName(),
		Projects: projs,
	})
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("failed to check for existing data source: %w", err)
	}
	if existing.ID != uuid.Nil {
		return nil, ErrDataSourceAlreadyExists
	}

	// Create data source record
	dsRecord, err := tx.CreateDataSource(ctx, db.CreateDataSourceParams{
		ProjectID:      projectID,
		Name:           ds.GetName(),
		DisplayName:    ds.GetName(),
		SubscriptionID: uuid.NullUUID{UUID: subscriptionID, Valid: subscriptionID != uuid.Nil},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create data source: %w", err)
	}

	// Create function records based on driver type
	if err := addDataSourceFunctions(ctx, tx, ds, dsRecord.ID, projectID); err != nil {
		return nil, fmt.Errorf("failed to create data source functions: %w", err)
	}

	if err := stx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}
	stx = nil // Don't try to rollback

	ds.Id = dsRecord.ID.String()

	return ds, nil
}

// Update updates an existing data source and its functions.
//
// Update handles data source modifications by using a transaction to ensure atomicity.
// We first validate and verify the data source exists, then update its basic info.
// For functions, we take a "delete and recreate" approach rather than individual updates
// because it's simpler and safer - it ensures consistency and avoids partial updates.
// All functions must use the same driver type to maintain data source integrity.
func (d *dataSourceService) Update(
	ctx context.Context,
	projectID uuid.UUID,
	subscriptionID uuid.UUID,
	ds *minderv1.DataSource,
	opts *Options,
) (*minderv1.DataSource, error) {
	if err := ds.Validate(); err != nil {
		return nil, fmt.Errorf("data source validation failed: %w", err)
	}

	stx, err := d.txBuilder(d, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to start transaction: %w", err)
	}

	defer func() {
		if stx == nil {
			// already committed
			return
		}
		if err := stx.Rollback(); err != nil {
			zerolog.Ctx(ctx).Error().Err(err).Msg("failed to rollback transaction")
		}
	}()

	tx := stx.Q()

	// Validate the subscription ID if present
	existingDS, err := getDataSourceFromDb(ctx, projectID, ReadBuilder().WithTransaction(tx), tx,
		func(ctx context.Context, tx db.ExtendQuerier, projs []uuid.UUID) (db.DataSource, error) {
			return getByNameQuery(ctx, tx, projs, ds.GetName())
		})
	if err != nil {
		return nil, fmt.Errorf("failed to get existing data source from DB: %w", err)
	}
	if err = namespaces.DoesSubscriptionIDMatch(subscriptionID, existingDS.SubscriptionID); err != nil {
		return nil, fmt.Errorf("failed to update data source: %w", err)
	}

	// Validate the data source functions update
	existingFunctions, err := getDataSourceFunctions(ctx, tx, existingDS)
	if err != nil {
		return nil, fmt.Errorf("failed to get existing data source functions: %w", err)
	}
	if err := validateDataSourceFunctionsUpdate(existingDS, existingFunctions, ds); err != nil {
		return nil, err
	}

	if _, err := tx.UpdateDataSource(ctx, db.UpdateDataSourceParams{
		ID:          existingDS.ID,
		ProjectID:   projectID,
		DisplayName: ds.GetName(),
	}); err != nil {
		return nil, fmt.Errorf("failed to update data source: %w", err)
	}

	if _, err := tx.DeleteDataSourceFunctions(ctx, db.DeleteDataSourceFunctionsParams{
		DataSourceID: existingDS.ID,
		ProjectID:    projectID,
	}); err != nil {
		return nil, fmt.Errorf("failed to delete existing functions: %w", err)
	}

	if err := addDataSourceFunctions(ctx, tx, ds, existingDS.ID, projectID); err != nil {
		return nil, fmt.Errorf("failed to create data source functions: %w", err)
	}

	if err := stx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}
	stx = nil // Don't try to rollback

	if ds.Id == "" {
		ds.Id = existingDS.ID.String()
	}

	return ds, nil
}

// Upsert creates the data source if it does not already exist
// or updates it if it already exists. This is used in the subscription
// logic.
func (d *dataSourceService) Upsert(
	ctx context.Context,
	projectID uuid.UUID,
	subscriptionID uuid.UUID,
	ds *minderv1.DataSource,
	opts *Options,
) error {
	// Simulate upsert semantics by trying to create, then trying to update.
	_, err := d.Create(ctx, projectID, subscriptionID, ds, opts)
	if err == nil {
		// Rule successfully created, we can stop here.
		return nil
	} else if !errors.Is(err, ErrDataSourceAlreadyExists) {
		return fmt.Errorf("error while creating data source: %w", err)
	}

	// If we get here: data source already exists. Let's update it.
	_, err = d.Update(ctx, projectID, subscriptionID, ds, opts)
	if err != nil {
		return fmt.Errorf("error while updating data source: %w", err)
	}
	return nil
}

// Delete deletes a data source in the given project.
func (d *dataSourceService) Delete(
	ctx context.Context, id uuid.UUID, project uuid.UUID, opts *Options) error {
	stx, err := d.txBuilder(d, opts)
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}
	defer func() {
		if stx == nil {
			// already committed
			return
		}
		if err := stx.Rollback(); err != nil {
			zerolog.Ctx(ctx).Error().Err(err).Msg("failed to rollback transaction")
		}
	}()

	// Get the transaction querier
	tx := stx.Q()

	// List rule types referencing the data source
	ret, err := tx.ListRuleTypesReferencesByDataSource(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to list rule types referencing data source %s: %w", id, err)
	}

	// Check if the data source is in use by any rule types
	if len(ret) > 0 {
		// Return an error with the rule types that are using the data source
		var existingRefs []string
		for _, r := range ret {
			existingRefs = append(existingRefs, r.RuleTypeID.String())
		}
		return util.UserVisibleError(codes.FailedPrecondition,
			"data source %s is in use by the following rule types: %v", id, existingRefs)
	}

	// We don't support the deletion of bundle data sources
	existingDS, err := getDataSourceFromDb(ctx, project, ReadBuilder().WithTransaction(tx), tx,
		func(ctx context.Context, tx db.ExtendQuerier, projs []uuid.UUID) (db.DataSource, error) {
			return getByIDQuery(ctx, tx, projs, id)
		})
	if err != nil {
		return fmt.Errorf("failed to get data source with id %s: %w", id, err)
	}
	if existingDS.SubscriptionID.Valid {
		return util.UserVisibleError(codes.FailedPrecondition,
			"data source %s cannot be deleted as it is part of a bundle", id)
	}

	// Delete the data source record
	_, err = tx.DeleteDataSource(ctx, db.DeleteDataSourceParams{
		ID:        id,
		ProjectID: project,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return util.UserVisibleError(codes.NotFound,
				"data source with id %s not found in project %s", id, project)
		}
		return fmt.Errorf("failed to delete data source with id %s: %w", id, err)
	}

	// Commit the transaction
	if err := stx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	stx = nil // Don't try to rollback
	return nil
}

// BuildDataSourceRegistry bundles up all data sources referenced in the rule type
// into a registry.
//
// Note that this assumes that the rule type has already been validated.
func (d *dataSourceService) BuildDataSourceRegistry(
	ctx context.Context, rt *minderv1.RuleType, opts *Options) (*v1datasources.DataSourceRegistry, error) {
	rawproj := rt.GetContext().GetProject()
	proj, err := uuid.Parse(rawproj)
	if err != nil {
		return nil, fmt.Errorf("failed to parse project UUID: %w", err)
	}

	instantiations := rt.GetDef().GetEval().GetDataSources()
	reg := v1datasources.NewDataSourceRegistry()

	// return early so we don't need to do useless work
	if len(instantiations) == 0 {
		return reg, nil
	}

	stx, err := d.txBuilder(d, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to start transaction: %w", err)
	}

	//nolint:gosec // we'll log this error later.
	defer stx.Rollback()

	tx := stx.Q()

	projectHierarchy, err := tx.GetParentProjects(ctx, proj)
	if err != nil {
		return nil, fmt.Errorf("failed to get project hierarchy: %w", err)
	}

	for _, ref := range instantiations {
		inst, err := d.instantiateDataSource(ctx, ref, projectHierarchy, tx)
		if err != nil {
			return nil, fmt.Errorf("failed to instantiate data source: %w", err)
		}

		impl, err := datasources.BuildFromProtobuf(inst)
		if err != nil {
			return nil, fmt.Errorf("failed to build data source from protobuf: %w", err)
		}

		if err := reg.RegisterDataSource(getDataSourceReferenceAlias(ref), impl); err != nil {
			return nil, fmt.Errorf("failed to register data source: %w", err)
		}
	}

	return reg, nil
}

// getDataSourceReferenceAlias gets the alias that the data source will be referred to by
// in the registry.
func getDataSourceReferenceAlias(dsr *minderv1.DataSourceReference) string {
	key := dsr.GetAlias()
	if key == "" {
		return dsr.GetName()
	}
	return key
}

// addDataSourceFunctions adds functions to a data source based on its driver type.
func addDataSourceFunctions(
	ctx context.Context,
	tx db.ExtendQuerier,
	ds *minderv1.DataSource,
	dsID uuid.UUID,
	projectID uuid.UUID,
) error {
	switch drv := ds.GetDriver().(type) {
	case *minderv1.DataSource_Structured:
		for name, def := range drv.Structured.GetDef() {
			defBytes, err := protojson.Marshal(def)
			if err != nil {
				return fmt.Errorf("failed to marshal structured data definition: %w", err)
			}

			if _, err := tx.AddDataSourceFunction(ctx, db.AddDataSourceFunctionParams{
				DataSourceID: dsID,
				ProjectID:    projectID,
				Name:         name,
				Type:         v1datasources.DataSourceDriverStruct,
				Definition:   defBytes,
			}); err != nil {
				return fmt.Errorf("failed to create data source function: %w", err)
			}
		}
	case *minderv1.DataSource_Rest:
		for name, def := range drv.Rest.GetDef() {
			defBytes, err := protojson.Marshal(def)
			if err != nil {
				return fmt.Errorf("failed to marshal REST definition: %w", err)
			}

			if _, err := tx.AddDataSourceFunction(ctx, db.AddDataSourceFunctionParams{
				DataSourceID: dsID,
				ProjectID:    projectID,
				Name:         name,
				Type:         v1datasources.DataSourceDriverRest,
				Definition:   defBytes,
			}); err != nil {
				return fmt.Errorf("failed to create data source function: %w", err)
			}
		}
	default:
		return fmt.Errorf("unsupported data source driver type: %T", drv)
	}
	return nil
}
