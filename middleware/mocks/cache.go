// Code generated by moq; DO NOT EDIT.
// github.com/matryer/moq

package mocks

import (
	"sync"
)

// CacheSvc is a mock implementation of cache.Service.
//
//	func TestSomethingThatUsesService(t *testing.T) {
//
//		// make and configure a mocked cache.Service
//		mockedService := &CacheSvc{
//			GetFunc: func(key string, fn func() (interface{}, error)) (interface{}, error) {
//				panic("mock out the Get method")
//			},
//		}
//
//		// use mockedService in code that requires cache.Service
//		// and then make assertions.
//
//	}
type CacheSvc struct {
	// GetFunc mocks the Get method.
	GetFunc func(key string, fn func() (interface{}, error)) (interface{}, error)

	// calls tracks calls to the methods.
	calls struct {
		// Get holds details about calls to the Get method.
		Get []struct {
			// Key is the key argument value.
			Key string
			// Fn is the fn argument value.
			Fn func() (interface{}, error)
		}
	}
	lockGet sync.RWMutex
}

// Get calls GetFunc.
func (mock *CacheSvc) Get(key string, fn func() (interface{}, error)) (interface{}, error) {
	if mock.GetFunc == nil {
		panic("CacheSvc.GetFunc: method is nil but Service.Get was just called")
	}
	callInfo := struct {
		Key string
		Fn  func() (interface{}, error)
	}{
		Key: key,
		Fn:  fn,
	}
	mock.lockGet.Lock()
	mock.calls.Get = append(mock.calls.Get, callInfo)
	mock.lockGet.Unlock()
	return mock.GetFunc(key, fn)
}

// GetCalls gets all the calls that were made to Get.
// Check the length with:
//
//	len(mockedService.GetCalls())
func (mock *CacheSvc) GetCalls() []struct {
	Key string
	Fn  func() (interface{}, error)
} {
	var calls []struct {
		Key string
		Fn  func() (interface{}, error)
	}
	mock.lockGet.RLock()
	calls = mock.calls.Get
	mock.lockGet.RUnlock()
	return calls
}

// ResetGetCalls reset all the calls that were made to Get.
func (mock *CacheSvc) ResetGetCalls() {
	mock.lockGet.Lock()
	mock.calls.Get = nil
	mock.lockGet.Unlock()
}

// ResetCalls reset all the calls that were made to all mocked methods.
func (mock *CacheSvc) ResetCalls() {
	mock.lockGet.Lock()
	mock.calls.Get = nil
	mock.lockGet.Unlock()
}
