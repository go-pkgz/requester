package mocks

import (
	"net/http"
	"sync/atomic"
)

type RoundTripper struct {
	RoundTripFunc func(*http.Request) (*http.Response, error)
	calls         int32
}

func (r *RoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	atomic.AddInt32(&r.calls, 1)
	return r.RoundTripFunc(req)
}

func (r *RoundTripper) Calls() int {
	return int(atomic.LoadInt32(&r.calls))
}
