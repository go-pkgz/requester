package cache

import "net/http"

// Methods sets what HTTP methods allowed to be cached, default is "GET" only
func Methods(methods ...string) func(m *Middleware) {
	return func(m *Middleware) {
		m.allowedMethods = append([]string{}, methods...)
	}
}

// KeyWithHeaders makes all headers to affect caching key
func KeyWithHeaders(m *Middleware) {
	m.keyComponents.headers.enabled = true
}

// KeyWithHeadersIncluded allows some headers to affect caching key
func KeyWithHeadersIncluded(headers ...string) func(m *Middleware) {
	return func(m *Middleware) {
		m.keyComponents.headers.enabled = true
		m.keyComponents.headers.include = append(m.keyComponents.headers.include, headers...)
	}
}

// KeyWithHeadersExcluded make all headers, except passed in to affect caching key
func KeyWithHeadersExcluded(headers ...string) func(m *Middleware) {
	return func(m *Middleware) {
		m.keyComponents.headers.enabled = true
		m.keyComponents.headers.exclude = append(m.keyComponents.headers.exclude, headers...)
	}
}

// KeyWithBody makes whole body to be a part of the caching key
func KeyWithBody(m *Middleware) {
	m.keyComponents.body = true
}

// KeyFunc defines custom caching key function
func KeyFunc(fn func(r *http.Request) string) func(m *Middleware) {
	return func(m *Middleware) {
		m.keyFunc = fn
	}
}
