package mocks

import (
	"net/http"
	"sync/atomic"
)

// RoundTripper mock to test other middlewares
type RoundTripper struct {
	RoundTripFunc func(*http.Request) (*http.Response, error)
	calls         int32
}

// RoundTrip adds to calls count and hit user-provided RoundTripFunc
func (r *RoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	atomic.AddInt32(&r.calls, 1)
	return r.RoundTripFunc(req)
}

// Calls returns how many time RoundTrip func called
func (r *RoundTripper) Calls() int {
	return int(atomic.LoadInt32(&r.calls))
}
