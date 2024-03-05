// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/juju/juju/internal/objectstore (interfaces: Claimer,ClaimExtender,HashFileSystemAccessor)
//
// Generated by this command:
//
//	mockgen -package objectstore -destination state_mock_test.go github.com/juju/juju/internal/objectstore Claimer,ClaimExtender,HashFileSystemAccessor
//

// Package objectstore is a generated GoMock package.
package objectstore

import (
	context "context"
	io "io"
	reflect "reflect"
	time "time"

	gomock "go.uber.org/mock/gomock"
)

// MockClaimer is a mock of Claimer interface.
type MockClaimer struct {
	ctrl     *gomock.Controller
	recorder *MockClaimerMockRecorder
}

// MockClaimerMockRecorder is the mock recorder for MockClaimer.
type MockClaimerMockRecorder struct {
	mock *MockClaimer
}

// NewMockClaimer creates a new mock instance.
func NewMockClaimer(ctrl *gomock.Controller) *MockClaimer {
	mock := &MockClaimer{ctrl: ctrl}
	mock.recorder = &MockClaimerMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockClaimer) EXPECT() *MockClaimerMockRecorder {
	return m.recorder
}

// Claim mocks base method.
func (m *MockClaimer) Claim(arg0 context.Context, arg1 string) (ClaimExtender, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Claim", arg0, arg1)
	ret0, _ := ret[0].(ClaimExtender)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Claim indicates an expected call of Claim.
func (mr *MockClaimerMockRecorder) Claim(arg0, arg1 any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Claim", reflect.TypeOf((*MockClaimer)(nil).Claim), arg0, arg1)
}

// Release mocks base method.
func (m *MockClaimer) Release(arg0 context.Context, arg1 string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Release", arg0, arg1)
	ret0, _ := ret[0].(error)
	return ret0
}

// Release indicates an expected call of Release.
func (mr *MockClaimerMockRecorder) Release(arg0, arg1 any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Release", reflect.TypeOf((*MockClaimer)(nil).Release), arg0, arg1)
}

// MockClaimExtender is a mock of ClaimExtender interface.
type MockClaimExtender struct {
	ctrl     *gomock.Controller
	recorder *MockClaimExtenderMockRecorder
}

// MockClaimExtenderMockRecorder is the mock recorder for MockClaimExtender.
type MockClaimExtenderMockRecorder struct {
	mock *MockClaimExtender
}

// NewMockClaimExtender creates a new mock instance.
func NewMockClaimExtender(ctrl *gomock.Controller) *MockClaimExtender {
	mock := &MockClaimExtender{ctrl: ctrl}
	mock.recorder = &MockClaimExtenderMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockClaimExtender) EXPECT() *MockClaimExtenderMockRecorder {
	return m.recorder
}

// Duration mocks base method.
func (m *MockClaimExtender) Duration() time.Duration {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Duration")
	ret0, _ := ret[0].(time.Duration)
	return ret0
}

// Duration indicates an expected call of Duration.
func (mr *MockClaimExtenderMockRecorder) Duration() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Duration", reflect.TypeOf((*MockClaimExtender)(nil).Duration))
}

// Extend mocks base method.
func (m *MockClaimExtender) Extend(arg0 context.Context) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Extend", arg0)
	ret0, _ := ret[0].(error)
	return ret0
}

// Extend indicates an expected call of Extend.
func (mr *MockClaimExtenderMockRecorder) Extend(arg0 any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Extend", reflect.TypeOf((*MockClaimExtender)(nil).Extend), arg0)
}

// MockHashFileSystemAccessor is a mock of HashFileSystemAccessor interface.
type MockHashFileSystemAccessor struct {
	ctrl     *gomock.Controller
	recorder *MockHashFileSystemAccessorMockRecorder
}

// MockHashFileSystemAccessorMockRecorder is the mock recorder for MockHashFileSystemAccessor.
type MockHashFileSystemAccessorMockRecorder struct {
	mock *MockHashFileSystemAccessor
}

// NewMockHashFileSystemAccessor creates a new mock instance.
func NewMockHashFileSystemAccessor(ctrl *gomock.Controller) *MockHashFileSystemAccessor {
	mock := &MockHashFileSystemAccessor{ctrl: ctrl}
	mock.recorder = &MockHashFileSystemAccessorMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockHashFileSystemAccessor) EXPECT() *MockHashFileSystemAccessorMockRecorder {
	return m.recorder
}

// DeleteByHash mocks base method.
func (m *MockHashFileSystemAccessor) DeleteByHash(arg0 context.Context, arg1 string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DeleteByHash", arg0, arg1)
	ret0, _ := ret[0].(error)
	return ret0
}

// DeleteByHash indicates an expected call of DeleteByHash.
func (mr *MockHashFileSystemAccessorMockRecorder) DeleteByHash(arg0, arg1 any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DeleteByHash", reflect.TypeOf((*MockHashFileSystemAccessor)(nil).DeleteByHash), arg0, arg1)
}

// GetByHash mocks base method.
func (m *MockHashFileSystemAccessor) GetByHash(arg0 context.Context, arg1 string) (io.ReadCloser, int64, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetByHash", arg0, arg1)
	ret0, _ := ret[0].(io.ReadCloser)
	ret1, _ := ret[1].(int64)
	ret2, _ := ret[2].(error)
	return ret0, ret1, ret2
}

// GetByHash indicates an expected call of GetByHash.
func (mr *MockHashFileSystemAccessorMockRecorder) GetByHash(arg0, arg1 any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetByHash", reflect.TypeOf((*MockHashFileSystemAccessor)(nil).GetByHash), arg0, arg1)
}

// HashExists mocks base method.
func (m *MockHashFileSystemAccessor) HashExists(arg0 context.Context, arg1 string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "HashExists", arg0, arg1)
	ret0, _ := ret[0].(error)
	return ret0
}

// HashExists indicates an expected call of HashExists.
func (mr *MockHashFileSystemAccessorMockRecorder) HashExists(arg0, arg1 any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "HashExists", reflect.TypeOf((*MockHashFileSystemAccessor)(nil).HashExists), arg0, arg1)
}
