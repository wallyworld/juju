// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/juju/juju/internal/charm/downloader (interfaces: Storage)
//
// Generated by this command:
//
//	mockgen -typed -package mocks -destination mocks/storage_mocks.go github.com/juju/juju/internal/charm/downloader Storage
//

// Package mocks is a generated GoMock package.
package mocks

import (
	context "context"
	reflect "reflect"

	downloader "github.com/juju/juju/internal/charm/downloader"
	gomock "go.uber.org/mock/gomock"
)

// MockStorage is a mock of Storage interface.
type MockStorage struct {
	ctrl     *gomock.Controller
	recorder *MockStorageMockRecorder
}

// MockStorageMockRecorder is the mock recorder for MockStorage.
type MockStorageMockRecorder struct {
	mock *MockStorage
}

// NewMockStorage creates a new mock instance.
func NewMockStorage(ctrl *gomock.Controller) *MockStorage {
	mock := &MockStorage{ctrl: ctrl}
	mock.recorder = &MockStorageMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockStorage) EXPECT() *MockStorageMockRecorder {
	return m.recorder
}

// PrepareToStoreCharm mocks base method.
func (m *MockStorage) PrepareToStoreCharm(arg0 string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "PrepareToStoreCharm", arg0)
	ret0, _ := ret[0].(error)
	return ret0
}

// PrepareToStoreCharm indicates an expected call of PrepareToStoreCharm.
func (mr *MockStorageMockRecorder) PrepareToStoreCharm(arg0 any) *MockStoragePrepareToStoreCharmCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "PrepareToStoreCharm", reflect.TypeOf((*MockStorage)(nil).PrepareToStoreCharm), arg0)
	return &MockStoragePrepareToStoreCharmCall{Call: call}
}

// MockStoragePrepareToStoreCharmCall wrap *gomock.Call
type MockStoragePrepareToStoreCharmCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockStoragePrepareToStoreCharmCall) Return(arg0 error) *MockStoragePrepareToStoreCharmCall {
	c.Call = c.Call.Return(arg0)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockStoragePrepareToStoreCharmCall) Do(f func(string) error) *MockStoragePrepareToStoreCharmCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockStoragePrepareToStoreCharmCall) DoAndReturn(f func(string) error) *MockStoragePrepareToStoreCharmCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}

// Store mocks base method.
func (m *MockStorage) Store(arg0 context.Context, arg1 string, arg2 downloader.DownloadedCharm) (string, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Store", arg0, arg1, arg2)
	ret0, _ := ret[0].(string)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Store indicates an expected call of Store.
func (mr *MockStorageMockRecorder) Store(arg0, arg1, arg2 any) *MockStorageStoreCall {
	mr.mock.ctrl.T.Helper()
	call := mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Store", reflect.TypeOf((*MockStorage)(nil).Store), arg0, arg1, arg2)
	return &MockStorageStoreCall{Call: call}
}

// MockStorageStoreCall wrap *gomock.Call
type MockStorageStoreCall struct {
	*gomock.Call
}

// Return rewrite *gomock.Call.Return
func (c *MockStorageStoreCall) Return(arg0 string, arg1 error) *MockStorageStoreCall {
	c.Call = c.Call.Return(arg0, arg1)
	return c
}

// Do rewrite *gomock.Call.Do
func (c *MockStorageStoreCall) Do(f func(context.Context, string, downloader.DownloadedCharm) (string, error)) *MockStorageStoreCall {
	c.Call = c.Call.Do(f)
	return c
}

// DoAndReturn rewrite *gomock.Call.DoAndReturn
func (c *MockStorageStoreCall) DoAndReturn(f func(context.Context, string, downloader.DownloadedCharm) (string, error)) *MockStorageStoreCall {
	c.Call = c.Call.DoAndReturn(f)
	return c
}
