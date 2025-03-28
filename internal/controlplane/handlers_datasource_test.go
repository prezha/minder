// SPDX-FileCopyrightText: Copyright 2024 The Minder Authors
// SPDX-License-Identifier: Apache-2.0

package controlplane

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	mockdb "github.com/mindersec/minder/database/mock"
	mock_service "github.com/mindersec/minder/internal/datasources/service/mock"
	"github.com/mindersec/minder/internal/db"
	"github.com/mindersec/minder/internal/engine/engcontext"
	minderv1 "github.com/mindersec/minder/pkg/api/protobuf/go/minder/v1"
)

func TestCreateDataSource(t *testing.T) {
	t.Parallel()

	projectID := uuid.New()
	tests := []struct {
		name              string
		setupMocks        func(*mock_service.MockDataSourcesService)
		request           *minderv1.CreateDataSourceRequest
		expectedResponse  *minderv1.CreateDataSourceResponse
		expectedErrorCode codes.Code
	}{
		{
			name: "happy path",
			setupMocks: func(dsService *mock_service.MockDataSourcesService) {
				dsService.EXPECT().
					Create(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&minderv1.DataSource{Name: "test-ds"}, nil)
			},
			request: &minderv1.CreateDataSourceRequest{
				DataSource: &minderv1.DataSource{Name: "test-ds"},
			},
			expectedResponse: &minderv1.CreateDataSourceResponse{
				DataSource: &minderv1.DataSource{Name: "test-ds"},
			},
			expectedErrorCode: codes.OK,
		},
		{
			name:              "missing data source",
			request:           &minderv1.CreateDataSourceRequest{},
			expectedResponse:  nil,
			expectedErrorCode: codes.InvalidArgument,
		},
	}

	for _, tt := range tests {
		tt := tt // capture range variable
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockStore := mockdb.NewMockStore(ctrl)
			// project validation
			mockStore.EXPECT().GetProjectByID(gomock.Any(), projectID).Return(db.Project{
				ID: projectID,
			}, nil).AnyTimes()

			mockDataSourceService := mock_service.NewMockDataSourcesService(ctrl)

			if tt.setupMocks != nil {
				tt.setupMocks(mockDataSourceService)
			}

			srv := newDefaultServer(t, mockStore, nil, nil, nil)
			srv.dataSourcesService = mockDataSourceService

			ctx := context.Background()
			ctx = engcontext.WithEntityContext(ctx, &engcontext.EntityContext{
				Project:  engcontext.Project{ID: projectID},
				Provider: engcontext.Provider{Name: "testing"},
			})

			resp, err := srv.CreateDataSource(ctx, tt.request)
			if tt.expectedErrorCode != codes.OK {
				require.Error(t, err)
				st, ok := status.FromError(err)
				require.True(t, ok)
				require.Equal(t, tt.expectedErrorCode, st.Code())
				require.Nil(t, resp)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expectedResponse, resp)
			}
		})
	}
}

func TestGetDataSourceById(t *testing.T) {
	t.Parallel()

	projectID := uuid.New()
	dsID := uuid.New()
	dsIDStr := dsID.String()
	tests := []struct {
		name              string
		setupMocks        func(*mock_service.MockDataSourcesService, *mockdb.MockStore)
		request           *minderv1.GetDataSourceByIdRequest
		expectedResponse  *minderv1.GetDataSourceByIdResponse
		expectedErrorCode codes.Code
	}{
		{
			name: "happy path",
			setupMocks: func(dsService *mock_service.MockDataSourcesService, mockStore *mockdb.MockStore) {
				mockStore.EXPECT().
					GetProjectByID(gomock.Any(), projectID).
					Return(db.Project{}, nil)
				dsService.EXPECT().
					GetByID(gomock.Any(), dsID, projectID, gomock.Any()).
					Return(&minderv1.DataSource{Id: dsIDStr, Name: "test-ds"}, nil)
			},
			request: &minderv1.GetDataSourceByIdRequest{
				Id: dsIDStr,
			},
			expectedResponse: &minderv1.GetDataSourceByIdResponse{
				DataSource: &minderv1.DataSource{Id: dsIDStr, Name: "test-ds"},
			},
			expectedErrorCode: codes.OK,
		},
		{
			name: "missing data source ID",
			setupMocks: func(_ *mock_service.MockDataSourcesService, _ *mockdb.MockStore) {
				// No need to set up mockStore expectations here
			},
			request:           &minderv1.GetDataSourceByIdRequest{},
			expectedResponse:  nil,
			expectedErrorCode: codes.InvalidArgument,
		},
		{
			name: "invalid data source ID format",
			setupMocks: func(_ *mock_service.MockDataSourcesService, _ *mockdb.MockStore) {
				// No need to set up mockStore expectations here
			},
			request: &minderv1.GetDataSourceByIdRequest{
				Id: "invalid-uuid",
			},
			expectedResponse:  nil,
			expectedErrorCode: codes.InvalidArgument,
		},
	}

	for _, tt := range tests {
		tt := tt // capture range variable
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockStore := mockdb.NewMockStore(ctrl)
			mockDataSourceService := mock_service.NewMockDataSourcesService(ctrl)

			if tt.setupMocks != nil {
				tt.setupMocks(mockDataSourceService, mockStore)
			}

			srv := newDefaultServer(t, mockStore, nil, nil, nil)
			srv.dataSourcesService = mockDataSourceService

			ctx := context.Background()
			ctx = engcontext.WithEntityContext(ctx, &engcontext.EntityContext{
				Project:  engcontext.Project{ID: projectID},
				Provider: engcontext.Provider{Name: "testing"},
			})

			resp, err := srv.GetDataSourceById(ctx, tt.request)
			if tt.expectedErrorCode != codes.OK {
				require.Error(t, err)
				st, ok := status.FromError(err)
				require.True(t, ok)
				require.Equal(t, tt.expectedErrorCode, st.Code())
				require.Nil(t, resp)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expectedResponse, resp)
			}
		})
	}
}

func TestGetDataSourceByName(t *testing.T) {
	t.Parallel()

	projectID := uuid.New()
	dsName := "test-ds"
	tests := []struct {
		name              string
		setupMocks        func(*mock_service.MockDataSourcesService, *mockdb.MockStore)
		request           *minderv1.GetDataSourceByNameRequest
		expectedResponse  *minderv1.GetDataSourceByNameResponse
		expectedErrorCode codes.Code
	}{
		{
			name: "happy path",
			setupMocks: func(dsService *mock_service.MockDataSourcesService, mockStore *mockdb.MockStore) {
				mockStore.EXPECT().
					GetProjectByID(gomock.Any(), projectID).
					Return(db.Project{}, nil)
				dsService.EXPECT().
					GetByName(gomock.Any(), dsName, projectID, gomock.Any()).
					Return(&minderv1.DataSource{Name: dsName}, nil)
			},
			request: &minderv1.GetDataSourceByNameRequest{
				Name: dsName,
			},
			expectedResponse: &minderv1.GetDataSourceByNameResponse{
				DataSource: &minderv1.DataSource{Name: dsName},
			},
			expectedErrorCode: codes.OK,
		},
		{
			name: "missing data source name",
			setupMocks: func(_ *mock_service.MockDataSourcesService, _ *mockdb.MockStore) {
				// No need to set up mockStore expectations here
			},
			request:           &minderv1.GetDataSourceByNameRequest{},
			expectedResponse:  nil,
			expectedErrorCode: codes.InvalidArgument,
		},
	}

	for _, tt := range tests {
		tt := tt // capture range variable
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockStore := mockdb.NewMockStore(ctrl)
			mockDataSourceService := mock_service.NewMockDataSourcesService(ctrl)

			if tt.setupMocks != nil {
				tt.setupMocks(mockDataSourceService, mockStore)
			}

			srv := newDefaultServer(t, mockStore, nil, nil, nil)
			srv.dataSourcesService = mockDataSourceService

			ctx := context.Background()
			ctx = engcontext.WithEntityContext(ctx, &engcontext.EntityContext{
				Project:  engcontext.Project{ID: projectID},
				Provider: engcontext.Provider{Name: "testing"},
			})

			resp, err := srv.GetDataSourceByName(ctx, tt.request)
			if tt.expectedErrorCode != codes.OK {
				require.Error(t, err)
				st, ok := status.FromError(err)
				require.True(t, ok)
				require.Equal(t, tt.expectedErrorCode, st.Code())
				require.Nil(t, resp)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expectedResponse, resp)
			}
		})
	}
}

func TestListDataSources(t *testing.T) {
	t.Parallel()

	projectID := uuid.New()
	dsList := []*minderv1.DataSource{
		{Name: "ds1"},
		{Name: "ds2"},
	}
	tests := []struct {
		name              string
		setupMocks        func(*mock_service.MockDataSourcesService, *mockdb.MockStore)
		expectedResponse  *minderv1.ListDataSourcesResponse
		expectedErrorCode codes.Code
	}{
		{
			name: "happy path",
			setupMocks: func(dsService *mock_service.MockDataSourcesService, mockStore *mockdb.MockStore) {
				mockStore.EXPECT().
					GetProjectByID(gomock.Any(), projectID).
					Return(db.Project{}, nil)
				dsService.EXPECT().
					List(gomock.Any(), projectID, gomock.Any()).
					Return(dsList, nil)
			},
			expectedResponse: &minderv1.ListDataSourcesResponse{
				DataSources: dsList,
			},
			expectedErrorCode: codes.OK,
		},
	}

	for _, tt := range tests {
		tt := tt // capture range variable
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockStore := mockdb.NewMockStore(ctrl)
			mockDataSourceService := mock_service.NewMockDataSourcesService(ctrl)

			if tt.setupMocks != nil {
				tt.setupMocks(mockDataSourceService, mockStore)
			}

			srv := newDefaultServer(t, mockStore, nil, nil, nil)
			srv.dataSourcesService = mockDataSourceService

			ctx := context.Background()
			ctx = engcontext.WithEntityContext(ctx, &engcontext.EntityContext{
				Project:  engcontext.Project{ID: projectID},
				Provider: engcontext.Provider{Name: "testing"},
			})

			resp, err := srv.ListDataSources(ctx, &minderv1.ListDataSourcesRequest{})
			if tt.expectedErrorCode != codes.OK {
				require.Error(t, err)
				st, ok := status.FromError(err)
				require.True(t, ok)
				require.Equal(t, tt.expectedErrorCode, st.Code())
				require.Nil(t, resp)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expectedResponse, resp)
			}
		})
	}
}

func TestUpdateDataSource(t *testing.T) {
	t.Parallel()

	projectID := uuid.New()
	dsID := uuid.New()
	dsIDStr := dsID.String()
	tests := []struct {
		name              string
		setupMocks        func(*mock_service.MockDataSourcesService, *mockdb.MockStore)
		request           *minderv1.UpdateDataSourceRequest
		expectedResponse  *minderv1.UpdateDataSourceResponse
		expectedErrorCode codes.Code
	}{
		{
			name: "happy path",
			setupMocks: func(dsService *mock_service.MockDataSourcesService, _ *mockdb.MockStore) {
				dsService.EXPECT().
					Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
					Return(&minderv1.DataSource{Id: dsIDStr, Name: "updated-ds"}, nil)
			},
			request: &minderv1.UpdateDataSourceRequest{
				DataSource: &minderv1.DataSource{Id: dsIDStr, Name: "updated-ds"},
			},
			expectedResponse: &minderv1.UpdateDataSourceResponse{
				DataSource: &minderv1.DataSource{Id: dsIDStr, Name: "updated-ds"},
			},
			expectedErrorCode: codes.OK,
		},
		{
			name:              "missing data source",
			request:           &minderv1.UpdateDataSourceRequest{},
			expectedResponse:  nil,
			expectedErrorCode: codes.InvalidArgument,
		},
	}

	for _, tt := range tests {
		tt := tt // capture range variable
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockStore := mockdb.NewMockStore(ctrl)
			// project validation
			mockStore.EXPECT().GetProjectByID(gomock.Any(), projectID).Return(db.Project{
				ID: projectID,
			}, nil).AnyTimes()

			mockDataSourceService := mock_service.NewMockDataSourcesService(ctrl)

			if tt.setupMocks != nil {
				tt.setupMocks(mockDataSourceService, mockStore)
			}

			srv := newDefaultServer(t, mockStore, nil, nil, nil)
			srv.dataSourcesService = mockDataSourceService

			ctx := context.Background()
			ctx = engcontext.WithEntityContext(ctx, &engcontext.EntityContext{
				Project:  engcontext.Project{ID: projectID},
				Provider: engcontext.Provider{Name: "testing"},
			})

			resp, err := srv.UpdateDataSource(ctx, tt.request)
			if tt.expectedErrorCode != codes.OK {
				require.Error(t, err)
				st, ok := status.FromError(err)
				require.True(t, ok)
				require.Equal(t, tt.expectedErrorCode, st.Code())
				require.Nil(t, resp)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expectedResponse, resp)
			}
		})
	}
}

func TestDeleteDataSourceById(t *testing.T) {
	t.Parallel()

	projectID := uuid.New()
	dsID := uuid.New()
	dsIDStr := dsID.String()
	tests := []struct {
		name              string
		setupMocks        func(*mock_service.MockDataSourcesService, *mockdb.MockStore)
		request           *minderv1.DeleteDataSourceByIdRequest
		expectedResponse  *minderv1.DeleteDataSourceByIdResponse
		expectedErrorCode codes.Code
	}{
		{
			name: "happy path",
			setupMocks: func(dsService *mock_service.MockDataSourcesService, mockStore *mockdb.MockStore) {
				mockStore.EXPECT().
					GetProjectByID(gomock.Any(), projectID).
					Return(db.Project{}, nil)
				dsService.EXPECT().
					Delete(gomock.Any(), dsID, projectID, gomock.Any()).
					Return(nil)
			},
			request: &minderv1.DeleteDataSourceByIdRequest{
				Id: dsIDStr,
			},
			expectedResponse: &minderv1.DeleteDataSourceByIdResponse{
				Id: dsIDStr,
			},
			expectedErrorCode: codes.OK,
		},
		{
			name:              "missing data source ID",
			request:           &minderv1.DeleteDataSourceByIdRequest{},
			expectedResponse:  nil,
			expectedErrorCode: codes.InvalidArgument,
		},
		{
			name: "invalid data source ID format",
			request: &minderv1.DeleteDataSourceByIdRequest{
				Id: "invalid-uuid",
			},
			expectedResponse:  nil,
			expectedErrorCode: codes.InvalidArgument,
		},
	}

	for _, tt := range tests {
		tt := tt // capture range variable
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockStore := mockdb.NewMockStore(ctrl)
			mockDataSourceService := mock_service.NewMockDataSourcesService(ctrl)

			if tt.setupMocks != nil {
				tt.setupMocks(mockDataSourceService, mockStore)
			}

			srv := newDefaultServer(t, mockStore, nil, nil, nil)
			srv.dataSourcesService = mockDataSourceService

			ctx := context.Background()
			ctx = engcontext.WithEntityContext(ctx, &engcontext.EntityContext{
				Project:  engcontext.Project{ID: projectID},
				Provider: engcontext.Provider{Name: "testing"},
			})

			resp, err := srv.DeleteDataSourceById(ctx, tt.request)
			if tt.expectedErrorCode != codes.OK {
				require.Error(t, err)
				st, ok := status.FromError(err)
				require.True(t, ok)
				require.Equal(t, tt.expectedErrorCode, st.Code())
				require.Nil(t, resp)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expectedResponse, resp)
			}
		})
	}
}

func TestDeleteDataSourceByName(t *testing.T) {
	t.Parallel()

	projectID := uuid.New()
	dsName := "test-ds"
	dsID := uuid.New()
	dsIDStr := dsID.String()
	tests := []struct {
		name              string
		setupMocks        func(*mock_service.MockDataSourcesService, *mockdb.MockStore)
		request           *minderv1.DeleteDataSourceByNameRequest
		expectedResponse  *minderv1.DeleteDataSourceByNameResponse
		expectedErrorCode codes.Code
	}{
		{
			name: "happy path",
			setupMocks: func(dsService *mock_service.MockDataSourcesService, mockStore *mockdb.MockStore) {
				mockStore.EXPECT().
					GetProjectByID(gomock.Any(), projectID).
					Return(db.Project{}, nil)
				dsService.EXPECT().
					GetByName(gomock.Any(), dsName, projectID, gomock.Any()).
					Return(&minderv1.DataSource{Id: dsIDStr, Name: dsName}, nil)
				dsService.EXPECT().
					Delete(gomock.Any(), dsID, projectID, gomock.Any()).
					Return(nil)
			},
			request: &minderv1.DeleteDataSourceByNameRequest{
				Name: dsName,
			},
			expectedResponse: &minderv1.DeleteDataSourceByNameResponse{
				Name: dsName,
			},
			expectedErrorCode: codes.OK,
		},
		{
			name:              "missing data source name",
			request:           &minderv1.DeleteDataSourceByNameRequest{},
			expectedResponse:  nil,
			expectedErrorCode: codes.InvalidArgument,
		},
		{
			name: "invalid data source ID format",
			setupMocks: func(dsService *mock_service.MockDataSourcesService, mockStore *mockdb.MockStore) {
				mockStore.EXPECT().
					GetProjectByID(gomock.Any(), projectID).
					Return(db.Project{}, nil)
				dsService.EXPECT().
					GetByName(gomock.Any(), dsName, projectID, gomock.Any()).
					Return(&minderv1.DataSource{Id: "invalid-uuid", Name: dsName}, nil)
			},
			request: &minderv1.DeleteDataSourceByNameRequest{
				Name: dsName,
			},
			expectedResponse:  nil,
			expectedErrorCode: codes.InvalidArgument,
		},
	}

	for _, tt := range tests {
		tt := tt // capture range variable
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockStore := mockdb.NewMockStore(ctrl)
			mockDataSourceService := mock_service.NewMockDataSourcesService(ctrl)

			if tt.setupMocks != nil {
				tt.setupMocks(mockDataSourceService, mockStore)
			}

			srv := newDefaultServer(t, mockStore, nil, nil, nil)
			srv.dataSourcesService = mockDataSourceService

			ctx := context.Background()
			ctx = engcontext.WithEntityContext(ctx, &engcontext.EntityContext{
				Project:  engcontext.Project{ID: projectID},
				Provider: engcontext.Provider{Name: "testing"},
			})

			resp, err := srv.DeleteDataSourceByName(ctx, tt.request)
			if tt.expectedErrorCode != codes.OK {
				require.Error(t, err)
				st, ok := status.FromError(err)
				require.True(t, ok)
				require.Equal(t, tt.expectedErrorCode, st.Code())
				require.Nil(t, resp)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expectedResponse, resp)
			}
		})
	}
}
