// SPDX-FileCopyrightText: Copyright 2023 The Minder Authors
// SPDX-License-Identifier: Apache-2.0

package engine_test

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/oauth2"
	"google.golang.org/protobuf/types/known/structpb"

	mockdb "github.com/mindersec/minder/database/mock"
	"github.com/mindersec/minder/internal/controlplane/metrics"
	"github.com/mindersec/minder/internal/crypto"
	"github.com/mindersec/minder/internal/db"
	"github.com/mindersec/minder/internal/engine"
	"github.com/mindersec/minder/internal/engine/actions/alert"
	"github.com/mindersec/minder/internal/engine/actions/remediate"
	"github.com/mindersec/minder/internal/engine/entities"
	"github.com/mindersec/minder/internal/entities/models"
	mockprops "github.com/mindersec/minder/internal/entities/properties/service/mock"
	mockhistory "github.com/mindersec/minder/internal/history/mock"
	"github.com/mindersec/minder/internal/logger"
	"github.com/mindersec/minder/internal/metrics/meters"
	"github.com/mindersec/minder/internal/providers"
	"github.com/mindersec/minder/internal/providers/github/clients"
	ghmanager "github.com/mindersec/minder/internal/providers/github/manager"
	ghService "github.com/mindersec/minder/internal/providers/github/service"
	"github.com/mindersec/minder/internal/providers/manager"
	"github.com/mindersec/minder/internal/providers/ratecache"
	"github.com/mindersec/minder/internal/providers/telemetry"
	minderv1 "github.com/mindersec/minder/pkg/api/protobuf/go/minder/v1"
	serverconfig "github.com/mindersec/minder/pkg/config/server"
	"github.com/mindersec/minder/pkg/engine/selectors"
	mock_selectors "github.com/mindersec/minder/pkg/engine/selectors/mock"
	"github.com/mindersec/minder/pkg/flags"
	"github.com/mindersec/minder/pkg/profiles"
	provinfv1 "github.com/mindersec/minder/pkg/providers/v1"
)

func TestExecutor_handleEntityEvent(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := mockdb.NewMockStore(ctrl)

	// declarations
	projectID := uuid.New()
	providerName := "github"
	providerID := uuid.New()
	passthroughRuleType := "passthrough"
	profileID := uuid.New()
	ruleTypeID := uuid.New()
	repositoryID := uuid.New()
	executionID := uuid.New()

	// write token key to file
	tmpdir := t.TempDir()
	tokenKeyPath := tmpdir + "/token_key"

	// generate 256-bit key
	key := make([]byte, 32)
	_, err := io.ReadFull(rand.Reader, key)
	require.NoError(t, err)
	encodedKey := base64.StdEncoding.EncodeToString(key)

	// write key to file
	err = os.WriteFile(tokenKeyPath, []byte(encodedKey), 0600)
	require.NoError(t, err, "expected no error")

	// Needed to keep these tests working as-is.
	// In future, beef up unit test coverage in the dependencies
	// of this code, and refactor these tests to use stubs.
	config := &serverconfig.Config{
		Auth: serverconfig.AuthConfig{TokenKey: tokenKeyPath},
	}
	cryptoEngine, err := crypto.NewEngineFromConfig(config)
	require.NoError(t, err)

	authtoken := generateFakeAccessToken(t, cryptoEngine)
	// -- start expectations
	// not valuable yet, but would have to be updated once actions start using this
	mockStore.EXPECT().GetRuleEvaluationByProfileIdAndRuleType(gomock.Any(),
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
	).Return(nil, nil)

	mockStore.EXPECT().
		GetProviderByID(gomock.Any(), gomock.Eq(providerID)).
		Return(db.Provider{
			ID:        providerID,
			Name:      providerName,
			ProjectID: projectID,
			Class:     db.ProviderClassGithub,
			Version:   provinfv1.V1,
			Implements: []db.ProviderType{
				db.ProviderTypeGithub,
			},
			Definition: json.RawMessage(`{"github": {}}`),
		}, nil).
		Times(2) // once for instantiating the provider, once to fill in selectors

	// get access token
	mockStore.EXPECT().
		GetAccessTokenByProjectID(gomock.Any(),
			db.GetAccessTokenByProjectIDParams{
				Provider:  providerName,
				ProjectID: projectID,
			}).
		Return(db.ProviderAccessToken{
			EncryptedAccessToken: authtoken,
		}, nil)

	// one rule for the profile
	ruleInstanceID := uuid.New()
	ruleParams := db.GetRuleInstancesEntityInProjectsParams{
		EntityType: db.EntitiesRepository,
		ProjectIds: []uuid.UUID{projectID},
	}
	mockStore.EXPECT().
		GetRuleInstancesEntityInProjects(gomock.Any(), ruleParams).
		Return([]db.RuleInstance{
			{
				ID:         ruleInstanceID,
				ProfileID:  profileID,
				RuleTypeID: ruleTypeID,
				Name:       passthroughRuleType,
				EntityType: db.EntitiesRepository,
				Def:        emptyJSON,
				Params:     emptyJSON,
				ProjectID:  projectID,
			}}, nil)

	evaluationID := uuid.New()
	historyService := mockhistory.NewMockEvaluationHistoryService(ctrl)
	historyService.EXPECT().
		StoreEvaluationStatus(
			gomock.Any(), gomock.Any(), ruleInstanceID, profileID, db.EntitiesRepository, repositoryID, gomock.Any(), gomock.Any()).
		Return(evaluationID, nil)

	mockStore.EXPECT().
		InsertRemediationEvent(gomock.Any(), db.InsertRemediationEventParams{
			EvaluationID: evaluationID,
			Status:       db.RemediationStatusTypesSkipped,
			Details:      "",
			Metadata:     json.RawMessage("{}"),
		}).
		Return(nil)

	mockStore.EXPECT().
		InsertAlertEvent(gomock.Any(), db.InsertAlertEventParams{
			EvaluationID: evaluationID,
			Status:       db.AlertStatusTypesSkipped,
			Details:      "",
			Metadata:     json.RawMessage("{}"),
		}).
		Return(nil)

	// only one project in the hierarchy
	mockStore.EXPECT().
		GetParentProjects(gomock.Any(), projectID).
		Return([]uuid.UUID{projectID}, nil).
		Times(2)

	// list one profile
	mockStore.EXPECT().
		BulkGetProfilesByID(gomock.Any(), []uuid.UUID{profileID}).
		Return([]db.BulkGetProfilesByIDRow{
			{
				Profile: db.Profile{
					ID:        profileID,
					Name:      "test-profile",
					ProjectID: projectID,
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
					Alert:     db.NullActionType{Valid: true, ActionType: db.ActionTypeOff},
					Remediate: db.NullActionType{Valid: true, ActionType: db.ActionTypeOff},
				},
				ProfilesWithSelectors: []db.ProfileSelector{
					{
						Entity: db.NullEntities{
							Valid:    true,
							Entities: db.EntitiesRepository,
						},
						Selector: "repository.name == 'foo/test'",
					},
				},
			},
		}, nil)

	// get relevant rule
	ruleTypeDef := &minderv1.RuleType_Definition{
		InEntity:   minderv1.RepositoryEntity.String(),
		RuleSchema: &structpb.Struct{},
		Ingest: &minderv1.RuleType_Definition_Ingest{
			Type: "builtin",
			Builtin: &minderv1.BuiltinType{
				Method: "Passthrough",
			},
		},
		Eval: &minderv1.RuleType_Definition_Eval{
			Type: "rego",
			Rego: &minderv1.RuleType_Definition_Eval_Rego{
				Type: "deny-by-default",
				Def: `package minder
default allow = true`,
			},
		},
	}

	marshalledRTD, err := json.Marshal(ruleTypeDef)
	require.NoError(t, err, "expected no error")

	mockStore.EXPECT().
		GetRuleTypeNameByID(gomock.Any(), gomock.Any()).
		Return(passthroughRuleType, nil)

	mockStore.EXPECT().
		GetRuleTypesByEntityInHierarchy(gomock.Any(), db.GetRuleTypesByEntityInHierarchyParams{
			EntityType: db.EntitiesRepository,
			Projects:   []uuid.UUID{projectID},
		}).
		Return([]db.RuleType{
			{
				ID:         ruleTypeID,
				Name:       passthroughRuleType,
				ProjectID:  projectID,
				Definition: marshalledRTD,
			},
		}, nil)

	// Mock update lease for lock
	mockStore.EXPECT().
		UpdateLease(gomock.Any(), db.UpdateLeaseParams{
			EntityInstanceID: repositoryID,
			LockedBy:         executionID,
		}).Return(nil)

	// Mock release lock
	mockStore.EXPECT().
		ReleaseLock(gomock.Any(), db.ReleaseLockParams{
			EntityInstanceID: repositoryID,
			LockedBy:         executionID,
		}).Return(nil)

	// -- end expectations

	ghProviderService := ghService.NewGithubProviderService(
		mockStore,
		cryptoEngine,
		metrics.NewNoopMetrics(),
		// These nil dependencies do not matter for the current tests
		nil,
		nil,
		clients.NewGitHubClientFactory(telemetry.NewNoopMetrics()),
	)

	propssvc := mockprops.NewMockPropertiesService(ctrl)

	githubProviderManager := ghmanager.NewGitHubProviderClassManager(
		&ratecache.NoopRestClientCache{},
		clients.NewGitHubClientFactory(telemetry.NewNoopMetrics()),
		&serverconfig.ProviderConfig{},
		&serverconfig.WebhookConfig{},
		nil,
		cryptoEngine,
		mockStore,
		ghProviderService,
		propssvc,
		metrics.NewNoopMetrics(),
		nil, // we won't be publishing events
	)

	providerStore := providers.NewProviderStore(mockStore)
	providerManager, closer, err := manager.NewProviderManager(context.Background(), providerStore, githubProviderManager)
	require.NoError(t, err)

	defer closer()

	execMetrics, err := engine.NewExecutorMetrics(&meters.NoopMeterFactory{})
	require.NoError(t, err)

	// stubbing related to evaluation history
	var txFunction func(querier db.ExtendQuerier) error
	mockStore.EXPECT().
		WithTransactionErr(gomock.AssignableToTypeOf(txFunction)).
		DoAndReturn(func(fn func(querier db.ExtendQuerier) error) error {
			return fn(mockStore)
		})

	mockSelection := mock_selectors.NewMockSelection(ctrl)
	mockSelection.EXPECT().
		Select(gomock.Any(), gomock.Any()).
		Return(true, "", nil).
		AnyTimes()

	mockSelectionBuilder := mock_selectors.NewMockSelectionBuilder(ctrl)
	mockSelectionBuilder.EXPECT().
		NewSelectionFromProfile(gomock.Any(), gomock.Any()).
		Return(mockSelection, nil).
		AnyTimes()

	mockPropSvc := mockprops.NewMockPropertiesService(ctrl)
	mockPropSvc.EXPECT().
		EntityWithPropertiesByID(gomock.Any(), repositoryID, gomock.Any()).
		Return(&models.EntityWithProperties{
			Entity: models.EntityInstance{
				ID:         repositoryID,
				Type:       minderv1.Entity_ENTITY_REPOSITORIES,
				Name:       "foo/test",
				ProjectID:  projectID,
				ProviderID: providerID,
			},
		}, nil)

	executor := engine.NewExecutor(
		mockStore,
		providerManager,
		execMetrics,
		historyService,
		&flags.FakeClient{},
		profiles.NewProfileStore(mockStore),
		selectors.NewEnv(),
		mockPropSvc,
	)

	eiw := entities.NewEntityInfoWrapper().
		WithProviderID(providerID).
		WithProjectID(projectID).
		WithRepository(&minderv1.Repository{
			Owner:    "foo",
			Name:     "test",
			RepoId:   123,
			CloneUrl: "github.com/foo/bar.git",
		}).WithID(repositoryID).
		WithExecutionID(executionID)

	ts := &logger.TelemetryStore{
		Project:    projectID,
		ProviderID: providerID,
		Repository: repositoryID,
	}
	ctx := ts.WithTelemetry(context.Background())

	err = executor.EvalEntityEvent(ctx, eiw)
	require.NoError(t, err)

	require.Len(t, ts.Evals, 1, "expected one eval to be logged")
	requredEval := ts.Evals[0]
	require.Equal(t, "test-profile", requredEval.Profile.Name)
	require.Equal(t, "success", requredEval.EvalResult)
	require.Equal(t, "passthrough", requredEval.RuleType.Name)
	require.Equal(t, "off", requredEval.Actions[alert.ActionType].State)
	require.Equal(t, "off", requredEval.Actions[remediate.ActionType].State)
}

func generateFakeAccessToken(t *testing.T, cryptoEngine crypto.Engine) pqtype.NullRawMessage {
	t.Helper()

	ftoken := &oauth2.Token{
		AccessToken:  "foo-bar",
		TokenType:    "bar-baz",
		RefreshToken: "",
		// Expires in 10 mins
		Expiry: time.Now().Add(10 * time.Minute),
	}

	// encrypt token
	encryptedToken, err := cryptoEngine.EncryptOAuthToken(ftoken)
	require.NoError(t, err)
	serialized, err := encryptedToken.Serialize()
	require.NoError(t, err)
	return pqtype.NullRawMessage{
		RawMessage: serialized,
		Valid:      true,
	}
}

var emptyJSON = []byte("{}")
