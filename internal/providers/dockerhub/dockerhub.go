// SPDX-FileCopyrightText: Copyright 2024 The Minder Authors
// SPDX-License-Identifier: Apache-2.0

// Package dockerhub provides a client for interacting with Docker Hub
package dockerhub

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"

	"golang.org/x/oauth2"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/mindersec/minder/internal/db"
	"github.com/mindersec/minder/internal/providers/oci"
	minderv1 "github.com/mindersec/minder/pkg/api/protobuf/go/minder/v1"
	"github.com/mindersec/minder/pkg/entities/properties"
	provifv1 "github.com/mindersec/minder/pkg/providers/v1"
)

// DockerHub is the string that represents the DockerHub provider
const DockerHub = "dockerhub"

const (
	dockerioBaseURL = "docker.io"
)

// Implements is the list of provider types that the DockerHub provider implements
var Implements = []db.ProviderType{
	db.ProviderTypeImageLister,
	db.ProviderTypeOci,
}

// AuthorizationFlows is the list of authorization flows that the DockerHub provider supports
var AuthorizationFlows = []db.AuthorizationFlow{
	db.AuthorizationFlowUserInput,
}

// dockerHubImageLister is the struct that contains the Docker Hub specific operations
type dockerHubImageLister struct {
	*oci.OCI
	cred      provifv1.OAuth2TokenCredential
	cli       *http.Client
	namespace string
	target    *url.URL
	cfg       *minderv1.DockerHubProviderConfig
}

// Ensure that the Docker Hub client implements the ImageLister interface
var _ provifv1.ImageLister = (*dockerHubImageLister)(nil)

// New creates a new Docker Hub client
func New(cred provifv1.OAuth2TokenCredential, cfg *minderv1.DockerHubProviderConfig) (*dockerHubImageLister, error) {
	cli := oauth2.NewClient(context.Background(), cred.GetAsOAuth2TokenSource())

	u, err := url.Parse("https://hub.docker.com/v2/repositories")
	if err != nil {
		return nil, fmt.Errorf("error parsing base URL: %w", err)
	}

	ns := cfg.GetNamespace()
	t := u.JoinPath(ns)

	o := oci.New(cred, dockerioBaseURL, path.Join(dockerioBaseURL, cfg.GetNamespace()))
	return &dockerHubImageLister{
		OCI:       o,
		namespace: ns,
		cred:      cred,
		cli:       cli,
		target:    t,
		cfg:       cfg,
	}, nil
}

type dhConfigWrapper struct {
	DockerHub *minderv1.DockerHubProviderConfig `json:"dockerhub" yaml:"dockerhub" mapstructure:"dockerhub" validate:"required"`
}

// ParseV1Config parses the raw config into a DockerHubProviderConfig struct
//
// TODO: This should be moved to a common location
func ParseV1Config(rawCfg json.RawMessage) (*minderv1.DockerHubProviderConfig, error) {
	var w dhConfigWrapper
	if err := provifv1.ParseAndValidate(rawCfg, &w); err != nil {
		return nil, err
	}

	// Validate the config according to the protobuf validation rules.
	if err := w.DockerHub.Validate(); err != nil {
		return nil, fmt.Errorf("error validating DockerHub v1 provider config: %w", err)
	}

	return w.DockerHub, nil
}

// MarshalV1Config marshals the DockerHubProviderConfig struct into a raw config
func MarshalV1Config(rawCfg json.RawMessage) (json.RawMessage, error) {
	var w dhConfigWrapper
	if err := json.Unmarshal(rawCfg, &w); err != nil {
		return nil, err
	}

	err := w.DockerHub.Validate()
	if err != nil {
		return nil, fmt.Errorf("error validating provider config: %w", err)
	}

	return json.Marshal(w)
}

func (d *dockerHubImageLister) GetNamespaceURL() string {
	return d.target.String()
}

// CanImplement returns true if the provider can implement the specified trait
func (*dockerHubImageLister) CanImplement(trait minderv1.ProviderType) bool {
	return trait == minderv1.ProviderType_PROVIDER_TYPE_IMAGE_LISTER ||
		trait == minderv1.ProviderType_PROVIDER_TYPE_OCI
}

// ListImages lists the containers in the Docker Hub
func (d *dockerHubImageLister) ListImages(ctx context.Context) ([]string, error) {
	req, err := http.NewRequest("GET", d.target.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	resp, err := d.cli.Do(req.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("error making request: %w", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusUnauthorized {
			return nil, fmt.Errorf("unauthorized: %s", resp.Status)
		}
		if resp.StatusCode == http.StatusNotFound {
			return nil, errors.New("not found")
		}
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// parse body
	toParse := struct {
		Results []struct {
			Name string `json:"name"`
		} `json:"results"`
	}{}

	if err := json.NewDecoder(resp.Body).Decode(&toParse); err != nil {
		return nil, fmt.Errorf("error decoding response: %w", err)
	}

	var containers []string
	for _, r := range toParse.Results {
		containers = append(containers, r.Name)
	}

	return containers, nil
}

// FetchAllProperties implements the provider interface
// TODO: Implement this
func (*dockerHubImageLister) FetchAllProperties(
	_ context.Context, _ *properties.Properties, _ minderv1.Entity, _ *properties.Properties,
) (*properties.Properties, error) {
	return nil, nil
}

// FetchProperty implements the provider interface
// TODO: Implement this
func (*dockerHubImageLister) FetchProperty(
	_ context.Context, _ *properties.Properties, _ minderv1.Entity, _ string) (*properties.Property, error) {
	return nil, nil
}

// GetEntityName implements the provider interface
// TODO: Implement this
func (*dockerHubImageLister) GetEntityName(_ minderv1.Entity, _ *properties.Properties) (string, error) {
	return "", nil
}

// SupportsEntity implements the Provider interface
func (*dockerHubImageLister) SupportsEntity(_ minderv1.Entity) bool {
	// TODO: implement
	return false
}

// RegisterEntity implements the Provider interface
func (*dockerHubImageLister) RegisterEntity(
	_ context.Context, _ minderv1.Entity, _ *properties.Properties,
) (*properties.Properties, error) {
	// TODO: implement
	return nil, nil
}

// DeregisterEntity implements the Provider interface
func (*dockerHubImageLister) DeregisterEntity(
	_ context.Context, _ minderv1.Entity, _ *properties.Properties,
) error {
	// TODO: implement
	return nil
}

// ReregisterEntity implements the Provider interface
func (*dockerHubImageLister) ReregisterEntity(
	_ context.Context, _ minderv1.Entity, _ *properties.Properties,
) error {
	// TODO: implement
	return nil
}

// PropertiesToProtoMessage implements the Provider interface
func (*dockerHubImageLister) PropertiesToProtoMessage(
	_ minderv1.Entity, _ *properties.Properties) (protoreflect.ProtoMessage, error) {
	// TODO: Implement
	return nil, nil
}
