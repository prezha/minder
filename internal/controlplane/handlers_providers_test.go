// SPDX-FileCopyrightText: Copyright 2024 The Minder Authors
// SPDX-License-Identifier: Apache-2.0

package controlplane

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/uuid"
	"github.com/lestrrat-go/jwx/v2/jwt/openid"
	"github.com/lib/pq"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/oauth2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"

	mockdb "github.com/mindersec/minder/database/mock"
	"github.com/mindersec/minder/internal/auth/jwt"
	"github.com/mindersec/minder/internal/authz/mock"
	"github.com/mindersec/minder/internal/controlplane/metrics"
	"github.com/mindersec/minder/internal/crypto"
	"github.com/mindersec/minder/internal/crypto/algorithms"
	mockcrypto "github.com/mindersec/minder/internal/crypto/mock"
	"github.com/mindersec/minder/internal/db"
	"github.com/mindersec/minder/internal/engine/engcontext"
	"github.com/mindersec/minder/internal/entities/models"
	propSvc "github.com/mindersec/minder/internal/entities/properties/service/mock"
	"github.com/mindersec/minder/internal/providers"
	"github.com/mindersec/minder/internal/providers/dockerhub"
	ghmanager "github.com/mindersec/minder/internal/providers/github/manager"
	mockgh "github.com/mindersec/minder/internal/providers/github/mock"
	mockprovsvc "github.com/mindersec/minder/internal/providers/github/service/mock"
	"github.com/mindersec/minder/internal/providers/manager"
	"github.com/mindersec/minder/internal/providers/ratecache"
	minder "github.com/mindersec/minder/pkg/api/protobuf/go/minder/v1"
	serverconfig "github.com/mindersec/minder/pkg/config/server"
	provinfv1 "github.com/mindersec/minder/pkg/providers/v1"
)

func newPbStruct(t *testing.T, data map[string]interface{}) *structpb.Struct {
	t.Helper()

	pbs, err := structpb.NewStruct(data)
	require.NoError(t, err)
	return pbs
}

type mockServer struct {
	server        *Server
	mockStore     *mockdb.MockStore
	mockGhService *mockprovsvc.MockGitHubProviderService
}

func testServer(t *testing.T, ctrl *gomock.Controller) *mockServer {
	t.Helper()

	mockStore := mockdb.NewMockStore(ctrl)
	mockCryptoEngine := mockcrypto.NewMockEngine(ctrl)
	providerStore := providers.NewProviderStore(mockStore)
	mockProvidersSvc := mockprovsvc.NewMockGitHubProviderService(ctrl)

	mockprops := propSvc.NewMockPropertiesService(ctrl)

	cancelable, cancel := context.WithCancel(context.Background())
	clientCache := ratecache.NewRestClientCache(cancelable)
	defer cancel()

	githubProviderManager := ghmanager.NewGitHubProviderClassManager(
		clientCache,
		nil,
		&serverconfig.ProviderConfig{},
		&serverconfig.WebhookConfig{},
		nil,
		mockCryptoEngine,
		mockStore,
		mockProvidersSvc,
		mockprops,
		metrics.NewNoopMetrics(),
		nil,
	)
	dockerhubProviderManager := dockerhub.NewDockerHubProviderClassManager(mockCryptoEngine, mockStore)

	providerManager, closer, err := manager.NewProviderManager(context.Background(), providerStore, githubProviderManager, dockerhubProviderManager)
	require.NoError(t, err)

	// We don't need the cache for these tests
	closer()

	authzClient := &mock.SimpleClient{
		Allowed: []uuid.UUID{uuid.New()},
	}

	server := Server{
		authzClient:     authzClient,
		cryptoEngine:    mockCryptoEngine,
		store:           mockStore,
		providerManager: providerManager,
		ghProviders:     mockProvidersSvc,
		cfg:             &serverconfig.Config{},
	}

	return &mockServer{
		server:        &server,
		mockStore:     mockStore,
		mockGhService: mockProvidersSvc,
	}
}

func TestCreateProvider(t *testing.T) {
	t.Parallel()

	scenarios := []struct {
		name          string
		providerClass db.ProviderClass
		userConfig    *structpb.Struct
		expected      minder.Provider
		expectedErr   string
	}{
		{
			name:          "test-github-defaults",
			providerClass: db.ProviderClassGithub,
			expected: minder.Provider{
				Name:   "test-github-defaults",
				Config: newPbStruct(t, map[string]interface{}{}),
				Class:  string(db.ProviderClassGithub),
			},
		},
		{
			name:          "test-github-config",
			providerClass: db.ProviderClassGithub,
			userConfig: newPbStruct(t, map[string]interface{}{
				"github": map[string]interface{}{
					"key":      "value", // will be ignored
					"endpoint": "my.little.github",
				},
			}),
			expected: minder.Provider{
				Name: "test-github-config",
				Config: newPbStruct(t, map[string]interface{}{
					"github": map[string]interface{}{
						"endpoint": "my.little.github",
					},
				}),
				Class: string(db.ProviderClassGithub),
			},
		},
		{
			name:          "test-github-app-defaults",
			providerClass: db.ProviderClassGithubApp,
			expected: minder.Provider{
				Name:   "test-github-app-defaults",
				Config: newPbStruct(t, map[string]interface{}{}),
				Class:  string(db.ProviderClassGithubApp),
			},
		},
		{
			name:          "test-github-app-config-newkey",
			providerClass: db.ProviderClassGithubApp,
			userConfig: newPbStruct(t, map[string]interface{}{
				"github_app": map[string]interface{}{
					"key":      "value", // will be ignored
					"endpoint": "my.little.github",
				},
			}),
			expected: minder.Provider{
				Name: "test-github-app-config-newkey",
				Config: newPbStruct(t, map[string]interface{}{
					"github_app": map[string]interface{}{
						"endpoint": "my.little.github",
					},
				}),
				Class: string(db.ProviderClassGithubApp),
			},
		},
		{
			name:          "test-github-app-config-oldkey",
			providerClass: db.ProviderClassGithubApp,
			userConfig: newPbStruct(t, map[string]interface{}{
				"github-app": map[string]interface{}{
					"key":      "value", // will be ignored
					"endpoint": "my.little.github",
				},
			}),
			expected: minder.Provider{
				Name: "test-github-app-config-oldkey",
				Config: newPbStruct(t, map[string]interface{}{
					"github_app": map[string]interface{}{
						"endpoint": "my.little.github",
					},
				}),
				Class: string(db.ProviderClassGithubApp),
			},
		},
		{
			name:          "test-dockerhub-config",
			providerClass: db.ProviderClassDockerhub,
			userConfig: newPbStruct(t, map[string]interface{}{
				"dockerhub": map[string]interface{}{
					"key":       "value", // will be ignored
					"namespace": "myproject",
				},
			}),
			expected: minder.Provider{
				Name: "test-dockerhub-config",
				Config: newPbStruct(t, map[string]interface{}{
					"dockerhub": map[string]interface{}{
						"namespace": "myproject",
					},
				}),
				Class: string(db.ProviderClassDockerhub),
			},
		},
	}

	for i := range scenarios {
		scenario := &scenarios[i]
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()

			projectID := uuid.New()
			projectIDStr := projectID.String()

			ctrl := gomock.NewController(t)
			t.Cleanup(ctrl.Finish)

			fakeServer := testServer(t, ctrl)
			require.NotNil(t, fakeServer)

			user := openid.New()
			assert.NoError(t, user.Set("sub", "testuser"))

			ctx := context.Background()
			ctx = jwt.WithAuthTokenContext(ctx, user)
			ctx = engcontext.WithEntityContext(ctx, &engcontext.EntityContext{
				Project:  engcontext.Project{ID: projectID},
				Provider: engcontext.Provider{Name: scenario.name},
			})

			jsonConfig, err := scenario.expected.Config.MarshalJSON()
			require.NoError(t, err)

			fakeServer.mockStore.EXPECT().CreateProvider(gomock.Any(), partialCreateParamsMatcher{
				t: t,
				value: db.CreateProviderParams{
					Name:       scenario.name,
					ProjectID:  projectID,
					Class:      scenario.providerClass,
					Definition: jsonConfig,
				}}).
				Return(db.Provider{
					Name: scenario.name,
				}, nil)

			fakeServer.mockStore.EXPECT().GetAccessTokenByProjectID(gomock.Any(), gomock.Any()).
				Return(db.ProviderAccessToken{}, sql.ErrNoRows)
			fakeServer.mockStore.EXPECT().GetInstallationIDByProviderID(gomock.Any(), gomock.Any()).
				Return(db.ProviderGithubAppInstallation{}, sql.ErrNoRows)

			resp, err := fakeServer.server.CreateProvider(ctx, &minder.CreateProviderRequest{
				Context: &minder.Context{
					Project:  &projectIDStr,
					Provider: &scenario.name,
				},
				Provider: &minder.Provider{
					Name:   scenario.name,
					Class:  string(scenario.providerClass),
					Config: scenario.userConfig,
				},
			})
			assert.NoError(t, err)

			// The config is tested by checking the expected parameters passed to the store
			assert.Equal(t, scenario.expected.Name, resp.GetProvider().GetName())
			assert.Equal(t, provinfv1.CredentialStateUnset, resp.GetProvider().GetCredentialsState())
		})
	}
}

func TestCreateProviderFailures(t *testing.T) {
	t.Parallel()

	t.Run("unknown-class", func(t *testing.T) {
		t.Parallel()

		projectID := uuid.New()
		projectIDStr := projectID.String()

		ctrl := gomock.NewController(t)
		t.Cleanup(ctrl.Finish)

		fakeServer := testServer(t, ctrl)
		providerName := "uknown-class-provider"

		_, err := fakeServer.server.CreateProvider(context.Background(), &minder.CreateProviderRequest{
			Context: &minder.Context{
				Project:  &projectIDStr,
				Provider: &providerName,
			},
			Provider: &minder.Provider{
				Name:  providerName,
				Class: "unknown-class",
			},
		})
		assert.Error(t, err)
		require.ErrorContains(t, err, "unexpected provider class")
	})

	t.Run("error-no-provider-param", func(t *testing.T) {
		t.Parallel()

		projectID := uuid.New()
		projectIDStr := projectID.String()

		ctrl := gomock.NewController(t)
		t.Cleanup(ctrl.Finish)

		fakeServer := testServer(t, ctrl)
		providerName := "test-provider-no-params"

		_, err := fakeServer.server.CreateProvider(context.Background(), &minder.CreateProviderRequest{
			Context: &minder.Context{
				Project:  &projectIDStr,
				Provider: &providerName,
			},
		})
		assert.Error(t, err)
		require.ErrorContains(t, err, "provider is required")
	})

	t.Run("provider-already-exists", func(t *testing.T) {
		t.Parallel()

		projectID := uuid.New()
		projectIDStr := projectID.String()

		ctrl := gomock.NewController(t)
		t.Cleanup(ctrl.Finish)

		fakeServer := testServer(t, ctrl)
		providerName := "test-provider-duplicate"

		user := openid.New()
		assert.NoError(t, user.Set("sub", "testuser"))

		ctx := context.Background()
		ctx = jwt.WithAuthTokenContext(ctx, user)
		ctx = engcontext.WithEntityContext(ctx, &engcontext.EntityContext{
			Project:  engcontext.Project{ID: projectID},
			Provider: engcontext.Provider{Name: providerName},
		})

		fakeServer.mockStore.EXPECT().CreateProvider(gomock.Any(), gomock.Any()).
			Return(db.Provider{}, &pq.Error{Code: "23505"}) // unique_violation

		resp, err := fakeServer.server.CreateProvider(ctx, &minder.CreateProviderRequest{
			Context: &minder.Context{
				Project:  &projectIDStr,
				Provider: &providerName,
			},
			Provider: &minder.Provider{
				Name:  providerName,
				Class: string(db.ProviderClassGithub),
			},
		})
		assert.Error(t, err)
		assert.Nil(t, resp)

		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.AlreadyExists, st.Code())
	})

	t.Run("dockerhub-does-not-validate", func(t *testing.T) {
		t.Parallel()

		projectID := uuid.New()
		projectIDStr := projectID.String()

		ctrl := gomock.NewController(t)
		t.Cleanup(ctrl.Finish)

		fakeServer := testServer(t, ctrl)
		providerName := "bad-dockerhub"

		_, err := fakeServer.server.CreateProvider(context.Background(), &minder.CreateProviderRequest{
			Context: &minder.Context{
				Project:  &projectIDStr,
				Provider: &providerName,
			},
			Provider: &minder.Provider{
				Name:  providerName,
				Class: string(db.ProviderClassDockerhub),
				Config: newPbStruct(t, map[string]interface{}{
					"dockerhub": map[string]interface{}{
						"key": "value",
					},
				}),
			},
		})
		assert.Error(t, err)
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
		require.ErrorContains(t, err, "namespace is required")
	})

	t.Run("github-app-does-not-validate", func(t *testing.T) {
		t.Parallel()

		projectID := uuid.New()
		projectIDStr := projectID.String()

		ctrl := gomock.NewController(t)
		t.Cleanup(ctrl.Finish)

		fakeServer := testServer(t, ctrl)
		providerName := "bad-github-app"

		_, err := fakeServer.server.CreateProvider(context.Background(), &minder.CreateProviderRequest{
			Context: &minder.Context{
				Project:  &projectIDStr,
				Provider: &providerName,
			},
			Provider: &minder.Provider{
				Name:  providerName,
				Class: string(db.ProviderClassGithubApp),
				Config: newPbStruct(t, map[string]interface{}{
					"auto_registration": map[string]interface{}{
						"entities": map[string]interface{}{
							"blah": map[string]interface{}{
								"enabled": true,
							},
						},
					},
					"github-app": map[string]interface{}{},
				}),
			},
		})
		assert.Error(t, err)
		require.ErrorContains(t, err, "error validating provider config: auto_registration: invalid entity type: blah")

		// test special-casing of the invalid config error
		st, ok := status.FromError(err)
		require.True(t, ok)
		assert.Equal(t, codes.InvalidArgument, st.Code())
		require.ErrorContains(t, err, "invalid provider config")
	})
}

type partialCreateParamsMatcher struct {
	value db.CreateProviderParams
	t     *testing.T
}

func (p partialCreateParamsMatcher) configMatches(name string, gotBytes json.RawMessage) bool {
	var exp, got interface{}

	if err := json.Unmarshal(p.value.Definition, &exp); err != nil {
		return false
	}
	if err := json.Unmarshal(gotBytes, &got); err != nil {
		return false
	}
	if !cmp.Equal(exp, got) {
		p.t.Logf("config mismatch for %s: %s\n", name, cmp.Diff(gotBytes, p.value.Definition))
		return false
	}
	return true
}

func (p partialCreateParamsMatcher) Matches(x interface{}) bool {
	typedX, ok := x.(db.CreateProviderParams)
	if !ok {
		return false
	}

	if !p.configMatches(p.value.Name, typedX.Definition) {
		return false
	}

	return cmp.Equal(typedX, p.value,
		cmpopts.IgnoreFields(db.CreateProviderParams{}, "Implements", "Definition", "AuthFlows"))
}

func (p partialCreateParamsMatcher) String() string {
	return fmt.Sprintf("partialCreateParamsMatcher %+v", p.value)
}

func TestDeleteProvider(t *testing.T) {
	t.Parallel()

	user := openid.New()
	assert.NoError(t, user.Set("sub", "testuser"))

	authzClient := &mock.SimpleClient{
		Allowed: []uuid.UUID{uuid.New()},
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	providerName := "test-provider"
	providerID := uuid.New()
	projectID := uuid.New()
	projectIDStr := projectID.String()
	accessToken := "test-token"

	mockProvidersSvc := mockprovsvc.NewMockGitHubProviderService(ctrl)
	mockProvidersSvc.EXPECT().DeleteInstallation(gomock.Any(), gomock.Any()).Return(nil)

	mockprops := propSvc.NewMockPropertiesService(ctrl)
	mockprops.EXPECT().
		EntityWithPropertiesByID(gomock.Any(), gomock.Any(), nil).
		Return(models.NewEntityWithPropertiesFromInstance(
			models.EntityInstance{}, nil), nil)

	mockCryptoEngine := mockcrypto.NewMockEngine(ctrl)
	mockCryptoEngine.EXPECT().
		DecryptOAuthToken(gomock.Any()).
		Return(oauth2.Token{AccessToken: accessToken}, nil).AnyTimes()

	mockStore := mockdb.NewMockStore(ctrl)
	mockStore.EXPECT().GetProviderByName(gomock.Any(), gomock.Any()).Return(db.Provider{
		Name:      providerName,
		ProjectID: projectID,
		Implements: []db.ProviderType{
			db.ProviderTypeGithub,
		},
		ID:         providerID,
		Version:    provinfv1.V1,
		Definition: json.RawMessage(`{"github": {}}`),
		Class:      db.ProviderClassGithub,
	}, nil)
	mockStore.EXPECT().
		GetAccessTokenByProjectID(gomock.Any(), gomock.Any()).
		Return(db.ProviderAccessToken{
			EncryptedAccessToken: generateSecret(t),
		}, nil).AnyTimes()
	mockStore.EXPECT().
		GetEntitiesByProvider(gomock.Any(), gomock.Any()).
		Return([]db.EntityInstance{
			{
				ID:         uuid.New(),
				Name:       "test-entity",
				ProviderID: providerID,
				ProjectID:  projectID,
			},
		}, nil).AnyTimes()
	mockStore.EXPECT().DeleteProvider(gomock.Any(), db.DeleteProviderParams{
		ID:        providerID,
		ProjectID: projectID,
	}).Return(nil)

	cancelable, cancel := context.WithCancel(context.Background())
	clientCache := ratecache.NewRestClientCache(cancelable)
	defer cancel()

	gh := mockgh.NewMockGitHub(ctrl)
	gh.EXPECT().DeregisterEntity(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

	clientCache.Set("", accessToken, db.ProviderTypeGithub, gh)

	// I am not replacing provider store with a stub since I want to reuse
	// these tests to test the logic in GitHubProviderClassManager
	providerStore := providers.NewProviderStore(mockStore)
	githubProviderManager := ghmanager.NewGitHubProviderClassManager(
		clientCache,
		nil,
		&serverconfig.ProviderConfig{},
		&serverconfig.WebhookConfig{},
		nil,
		mockCryptoEngine,
		mockStore,
		mockProvidersSvc,
		mockprops,
		metrics.NewNoopMetrics(),
		nil,
	)
	ctx := context.Background()
	providerManager, closer, err := manager.NewProviderManager(context.Background(), providerStore, githubProviderManager)
	require.NoError(t, err)

	// We don't need the cache for these tests
	closer()

	server := Server{
		cryptoEngine:    mockCryptoEngine,
		store:           mockStore,
		ghProviders:     mockProvidersSvc,
		authzClient:     authzClient,
		providerStore:   providerStore,
		providerManager: providerManager,
		cfg:             &serverconfig.Config{},
	}

	ctx = jwt.WithAuthTokenContext(ctx, user)
	ctx = engcontext.WithEntityContext(ctx, &engcontext.EntityContext{
		Project:  engcontext.Project{ID: projectID},
		Provider: engcontext.Provider{Name: providerName},
	})

	resp, err := server.DeleteProvider(ctx, &minder.DeleteProviderRequest{
		Context: &minder.Context{
			Project:  &projectIDStr,
			Provider: &providerName,
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, providerName, resp.Name)
}

func TestDeleteProviderByID(t *testing.T) {
	t.Parallel()

	user := openid.New()
	assert.NoError(t, user.Set("sub", "testuser"))

	authzClient := &mock.SimpleClient{
		Allowed: []uuid.UUID{uuid.New()},
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	providerName := "test-provider"
	providerID := uuid.New()
	projectID := uuid.New()
	projectIDStr := projectID.String()
	accessToken := "test-token"

	mockProvidersSvc := mockprovsvc.NewMockGitHubProviderService(ctrl)
	mockProvidersSvc.EXPECT().DeleteInstallation(gomock.Any(), gomock.Any()).Return(nil)

	mockprops := propSvc.NewMockPropertiesService(ctrl)
	mockprops.EXPECT().
		EntityWithPropertiesByID(gomock.Any(), gomock.Any(), nil).
		Return(models.NewEntityWithPropertiesFromInstance(
			models.EntityInstance{}, nil), nil)

	mockCryptoEngine := mockcrypto.NewMockEngine(ctrl)
	mockCryptoEngine.EXPECT().
		DecryptOAuthToken(gomock.Any()).
		Return(oauth2.Token{AccessToken: accessToken}, nil).AnyTimes()

	mockStore := mockdb.NewMockStore(ctrl)
	params := db.GetProviderByIDAndProjectParams{
		ID:        providerID,
		ProjectID: projectID,
	}
	mockStore.EXPECT().GetProviderByIDAndProject(gomock.Any(), params).Return(db.Provider{
		Name:      providerName,
		ID:        providerID,
		ProjectID: projectID,
		Implements: []db.ProviderType{
			db.ProviderTypeGithub,
		},
		Version:    provinfv1.V1,
		Definition: json.RawMessage(`{"github": {}}`),
		Class:      db.ProviderClassGithub,
	}, nil)
	mockStore.EXPECT().DeleteProvider(gomock.Any(), db.DeleteProviderParams{
		ID:        providerID,
		ProjectID: projectID,
	}).Return(nil)
	mockStore.EXPECT().
		GetAccessTokenByProjectID(gomock.Any(), gomock.Any()).
		Return(db.ProviderAccessToken{
			EncryptedAccessToken: generateSecret(t),
		}, nil).AnyTimes()
	mockStore.EXPECT().
		GetEntitiesByProvider(gomock.Any(), gomock.Any()).
		Return([]db.EntityInstance{
			{
				ID:         uuid.New(),
				Name:       "test-entity",
				ProviderID: providerID,
				ProjectID:  projectID,
			},
		}, nil).AnyTimes()

	cancelable, cancel := context.WithCancel(context.Background())
	clientCache := ratecache.NewRestClientCache(cancelable)
	defer cancel()

	gh := mockgh.NewMockGitHub(ctrl)
	gh.EXPECT().DeregisterEntity(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

	clientCache.Set("", accessToken, db.ProviderTypeGithub, gh)

	providerStore := providers.NewProviderStore(mockStore)
	githubProviderManager := ghmanager.NewGitHubProviderClassManager(
		clientCache,
		nil,
		&serverconfig.ProviderConfig{},
		&serverconfig.WebhookConfig{},
		nil,
		mockCryptoEngine,
		mockStore,
		mockProvidersSvc,
		mockprops,
		metrics.NewNoopMetrics(),
		nil,
	)
	ctx := context.Background()
	providerManager, closer, err := manager.NewProviderManager(context.Background(), providerStore, githubProviderManager)
	require.NoError(t, err)

	// We don't need the cache for these tests
	closer()

	server := Server{
		cryptoEngine:    mockCryptoEngine,
		store:           mockStore,
		ghProviders:     mockProvidersSvc,
		authzClient:     authzClient,
		providerStore:   providerStore,
		providerManager: providerManager,
		cfg:             &serverconfig.Config{},
	}

	ctx = jwt.WithAuthTokenContext(ctx, user)
	ctx = engcontext.WithEntityContext(ctx, &engcontext.EntityContext{
		Project: engcontext.Project{ID: projectID},
	})

	resp, err := server.DeleteProviderByID(ctx, &minder.DeleteProviderByIDRequest{
		Context: &minder.Context{
			Project: &projectIDStr,
		},
		Id: providerID.String(),
	})
	assert.NoError(t, err)
	assert.Equal(t, providerID.String(), resp.Id)
}

func generateSecret(t *testing.T) pqtype.NullRawMessage {
	t.Helper()

	data := crypto.EncryptedData{
		Algorithm: algorithms.Aes256Gcm,
		// randomly generated
		EncodedData: "dnS6VFiMYrfnbeP6eixmBw==",
		KeyVersion:  "",
	}

	serialized, err := data.Serialize()
	require.NoError(t, err)

	return pqtype.NullRawMessage{
		RawMessage: serialized,
		Valid:      true,
	}
}
