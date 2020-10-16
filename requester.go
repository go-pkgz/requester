// Package requester helps to make request and run it.
// Adds predefined json headers and auth token.
// Allows to run with optional repeater, circuit breaker and cache.
package requester

import (
	"net/http"

	"github.com/go-pkgz/requester/middleware"
)

// Requester provides a wrapper for the standard http.Do request.
type Requester struct {
	client      http.Client
	middlewares []middleware.RoundTripperHandler
}

// New creates requester with defaults
func New(client http.Client, middlewares ...middleware.RoundTripperHandler) *Requester {
	return &Requester{
		client:      client,
		middlewares: middlewares,
	}
}

// Use adds middleware(s) to the requester chain
func (r *Requester) Use(middlewares ...middleware.RoundTripperHandler) {
	r.middlewares = append(r.middlewares, middlewares...)
}

// With makes a new Requested with inherited middlewares and add passed middleware(s) to the chain
func (r *Requester) With(middlewares ...middleware.RoundTripperHandler) *Requester {
	res := &Requester{
		client:      r.client,
		middlewares: append(r.middlewares, middlewares...),
	}
	return res
}

// Client returns http.Client with all middlewares injected
func (r *Requester) Client() *http.Client {
	r.client.Transport = http.DefaultTransport
	for _, handler := range r.middlewares {
		r.client.Transport = handler(r.client.Transport)
	}
	return &r.client
}

// Do runs http request with optional middleware handlers wrapping the request
func (r *Requester) Do(req *http.Request) (*http.Response, error) {

	r.client.Transport = http.DefaultTransport
	for _, handler := range r.middlewares {
		r.client.Transport = handler(r.client.Transport)
	}
	return r.client.Do(req)
}
