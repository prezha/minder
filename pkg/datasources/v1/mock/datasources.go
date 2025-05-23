// Code generated by MockGen. DO NOT EDIT.
// Source: ./datasources.go
//
// Generated by this command:
//
//	mockgen -package mock_v1 -destination=./mock/datasources.go -source=./datasources.go
//

// Package mock_v1 is a generated GoMock package.
package mock_v1

import (
	context "context"
	reflect "reflect"

	v1 "github.com/mindersec/minder/pkg/datasources/v1"
	interfaces "github.com/mindersec/minder/pkg/engine/v1/interfaces"
	gomock "go.uber.org/mock/gomock"
	structpb "google.golang.org/protobuf/types/known/structpb"
)

// MockDataSourceFuncDef is a mock of DataSourceFuncDef interface.
type MockDataSourceFuncDef struct {
	ctrl     *gomock.Controller
	recorder *MockDataSourceFuncDefMockRecorder
	isgomock struct{}
}

// MockDataSourceFuncDefMockRecorder is the mock recorder for MockDataSourceFuncDef.
type MockDataSourceFuncDefMockRecorder struct {
	mock *MockDataSourceFuncDef
}

// NewMockDataSourceFuncDef creates a new mock instance.
func NewMockDataSourceFuncDef(ctrl *gomock.Controller) *MockDataSourceFuncDef {
	mock := &MockDataSourceFuncDef{ctrl: ctrl}
	mock.recorder = &MockDataSourceFuncDefMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockDataSourceFuncDef) EXPECT() *MockDataSourceFuncDefMockRecorder {
	return m.recorder
}

// Call mocks base method.
func (m *MockDataSourceFuncDef) Call(ctx context.Context, ingest *interfaces.Ingested, args any) (any, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Call", ctx, ingest, args)
	ret0, _ := ret[0].(any)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Call indicates an expected call of Call.
func (mr *MockDataSourceFuncDefMockRecorder) Call(ctx, ingest, args any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Call", reflect.TypeOf((*MockDataSourceFuncDef)(nil).Call), ctx, ingest, args)
}

// GetArgsSchema mocks base method.
func (m *MockDataSourceFuncDef) GetArgsSchema() *structpb.Struct {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetArgsSchema")
	ret0, _ := ret[0].(*structpb.Struct)
	return ret0
}

// GetArgsSchema indicates an expected call of GetArgsSchema.
func (mr *MockDataSourceFuncDefMockRecorder) GetArgsSchema() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetArgsSchema", reflect.TypeOf((*MockDataSourceFuncDef)(nil).GetArgsSchema))
}

// ValidateArgs mocks base method.
func (m *MockDataSourceFuncDef) ValidateArgs(obj any) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ValidateArgs", obj)
	ret0, _ := ret[0].(error)
	return ret0
}

// ValidateArgs indicates an expected call of ValidateArgs.
func (mr *MockDataSourceFuncDefMockRecorder) ValidateArgs(obj any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ValidateArgs", reflect.TypeOf((*MockDataSourceFuncDef)(nil).ValidateArgs), obj)
}

// ValidateUpdate mocks base method.
func (m *MockDataSourceFuncDef) ValidateUpdate(obj *structpb.Struct) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ValidateUpdate", obj)
	ret0, _ := ret[0].(error)
	return ret0
}

// ValidateUpdate indicates an expected call of ValidateUpdate.
func (mr *MockDataSourceFuncDefMockRecorder) ValidateUpdate(obj any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ValidateUpdate", reflect.TypeOf((*MockDataSourceFuncDef)(nil).ValidateUpdate), obj)
}

// MockDataSource is a mock of DataSource interface.
type MockDataSource struct {
	ctrl     *gomock.Controller
	recorder *MockDataSourceMockRecorder
	isgomock struct{}
}

// MockDataSourceMockRecorder is the mock recorder for MockDataSource.
type MockDataSourceMockRecorder struct {
	mock *MockDataSource
}

// NewMockDataSource creates a new mock instance.
func NewMockDataSource(ctrl *gomock.Controller) *MockDataSource {
	mock := &MockDataSource{ctrl: ctrl}
	mock.recorder = &MockDataSourceMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockDataSource) EXPECT() *MockDataSourceMockRecorder {
	return m.recorder
}

// GetFuncs mocks base method.
func (m *MockDataSource) GetFuncs() map[v1.DataSourceFuncKey]v1.DataSourceFuncDef {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetFuncs")
	ret0, _ := ret[0].(map[v1.DataSourceFuncKey]v1.DataSourceFuncDef)
	return ret0
}

// GetFuncs indicates an expected call of GetFuncs.
func (mr *MockDataSourceMockRecorder) GetFuncs() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetFuncs", reflect.TypeOf((*MockDataSource)(nil).GetFuncs))
}
