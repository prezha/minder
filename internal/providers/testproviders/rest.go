// SPDX-FileCopyrightText: Copyright 2024 The Minder Authors
// SPDX-License-Identifier: Apache-2.0

package testproviders

import (
	"context"

	"github.com/mindersec/minder/internal/providers/http"
	"github.com/mindersec/minder/internal/providers/noop"
	"github.com/mindersec/minder/internal/providers/telemetry"
	minderv1 "github.com/mindersec/minder/pkg/api/protobuf/go/minder/v1"
	"github.com/mindersec/minder/pkg/entities/properties"
	provifv1 "github.com/mindersec/minder/pkg/providers/v1"
)

// RESTProvider is a test implementation of the REST provider
// interface
type RESTProvider struct {
	noop.Provider
	*http.REST
}

// NewRESTProvider creates a new REST provider
func NewRESTProvider(
	config *minderv1.RESTProviderConfig,
	metrics telemetry.HttpClientMetrics,
	credential provifv1.RestCredential,
) (*RESTProvider, error) {
	r, err := http.NewREST(config, metrics, credential)
	if err != nil {
		return nil, err
	}
	return &RESTProvider{
		REST: r,
	}, nil
}

// Ensure RESTProvider implements the Provider interface
var _ provifv1.Provider = (*RESTProvider)(nil)

// CanImplement implements the Provider interface
func (*RESTProvider) CanImplement(trait minderv1.ProviderType) bool {
	return trait == minderv1.ProviderType_PROVIDER_TYPE_REST
}

// FetchAllProperties implements the Provider interface
func (*RESTProvider) FetchAllProperties(
	_ context.Context, _ *properties.Properties, _ minderv1.Entity, _ *properties.Properties,
) (*properties.Properties, error) {
	return nil, nil
}

// FetchProperty implements the Provider interface
func (*RESTProvider) FetchProperty(
	_ context.Context, _ *properties.Properties, _ minderv1.Entity, _ string) (*properties.Property, error) {
	return nil, nil
}

// GetEntityName implements the Provider interface
func (*RESTProvider) GetEntityName(_ minderv1.Entity, _ *properties.Properties) (string, error) {
	return "", nil
}

// SupportsEntity implements the Provider interface
func (*RESTProvider) SupportsEntity(_ minderv1.Entity) bool {
	// TODO: implement
	return false
}

// RegisterEntity implements the Provider interface
func (*RESTProvider) RegisterEntity(
	_ context.Context, _ minderv1.Entity, _ *properties.Properties,
) (*properties.Properties, error) {
	// TODO: implement
	return nil, nil
}

// DeregisterEntity implements the Provider interface
func (*RESTProvider) DeregisterEntity(_ context.Context, _ minderv1.Entity, _ *properties.Properties) error {
	// TODO: implement
	return nil
}

// ReregisterEntity implements the Provider interface
func (*RESTProvider) ReregisterEntity(
	_ context.Context, _ minderv1.Entity, _ *properties.Properties,
) error {
	// TODO: implement
	return nil
}
