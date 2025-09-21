// Package cache implements middleware for response caching. Request's component used as a key.
package cache

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"sort"
	"strings"

	"github.com/go-pkgz/requester/middleware"
)

// Middleware for caching responses. The key cache generated from request.
type Middleware struct {
	Service

	allowedMethods []string
	keyFunc        func(r *http.Request) string
	keyComponents  struct {
		body    bool
		headers struct {
			enabled bool
			include []string
			exclude []string
		}
	}

	dbg bool
}

const maxBodySize = 1024 * 16

// Service defines loading cache interface to be used for caching, matching github.com/go-pkgz/lcw interface
type Service interface {
	Get(key string, fn func() (interface{}, error)) (interface{}, error)
}

// ServiceFunc is an adapter to allow the use of an ordinary functions as the loading cache.
type ServiceFunc func(key string, fn func() (interface{}, error)) (interface{}, error)

// Get and/or fill the cached value for a given keyDbg
func (c ServiceFunc) Get(key string, fn func() (interface{}, error)) (interface{}, error) {
	return c(key, fn)
}

// New makes cache middleware for given cache.Service and optional set of params
// By default allowed methods limited to GET only and key for request's URL
func New(svc Service, opts ...func(m *Middleware)) *Middleware {
	res := Middleware{Service: svc, allowedMethods: []string{"GET"}}
	for _, opt := range opts {
		opt(&res)
	}
	return &res
}

// Middleware is the middleware wrapper injecting external cache.Service (LoadingCache) into the call chain.
// Key extracted from the request and options defines what part of request should be used for key and what method
// are allowed for caching.
func (m *Middleware) Middleware(next http.RoundTripper) http.RoundTripper {
	fn := func(req *http.Request) (resp *http.Response, err error) {

		if m.Service == nil || !m.methodCacheable(req) {
			return next.RoundTrip(req)
		}

		key, e := m.extractCacheKey(req)
		if e != nil {
			return nil, fmt.Errorf("cache key: %w", e)
		}

		cachedResp, e := m.Get(key, func() (interface{}, error) {
			resp, err = next.RoundTrip(req)
			if err != nil {
				return nil, fmt.Errorf("cache: transport error: %w", err)
			}
			if resp.Body == nil {
				return nil, nil
			}
			return httputil.DumpResponse(resp, true)
		})

		if e != nil {
			return nil, fmt.Errorf("cache read for %s: %w", key, e)
		}

		body := cachedResp.([]byte)
		return http.ReadResponse(bufio.NewReader(bytes.NewReader(body)), req)
	}
	return middleware.RoundTripperFunc(fn)
}

func (m *Middleware) extractCacheKey(req *http.Request) (key string, err error) {
	bodyKey := func() (string, error) {
		if req.Body == nil {
			return "", nil
		}
		reqBody, e := io.ReadAll(io.LimitReader(req.Body, maxBodySize))
		if e != nil {
			return "", fmt.Errorf("cache: failed to read body: %w", e)
		}
		_ = req.Body.Close()
		req.Body = io.NopCloser(bytes.NewReader(reqBody))
		return string(reqBody), nil
	}

	bkey := ""
	if m.keyComponents.body && m.keyFunc == nil {
		bkey, err = bodyKey()
	}

	hkey := ""
	if m.keyComponents.headers.enabled && m.keyFunc == nil {
		var hh []string
		for k, h := range req.Header {
			if m.headerAllowed(k) {
				hh = append(hh, k+":"+strings.Join(h, "%%"))
			}
		}
		sort.Strings(hh)
		hkey = strings.Join(hh, "$$")
	}

	if m.keyFunc != nil {
		key = m.keyFunc(req)
	} else {
		key = fmt.Sprintf("%s##%s##%v##%s", req.URL.String(), req.Method, hkey, bkey)
	}
	if m.dbg { // dbg for testing only, keeps the key human-readable
		return key, nil
	}

	return fmt.Sprintf("%x", sha256.Sum256([]byte(key))), err
}

func (m *Middleware) headerAllowed(key string) bool {
	if !m.keyComponents.headers.enabled {
		return false
	}
	if len(m.keyComponents.headers.include) > 0 {
		for _, h := range m.keyComponents.headers.include {
			if strings.EqualFold(key, h) {
				return true
			}
		}
		return false
	}

	if len(m.keyComponents.headers.exclude) > 0 {
		for _, h := range m.keyComponents.headers.exclude {
			if strings.EqualFold(key, h) {
				return false
			}
		}
		return true
	}
	return true
}

func (m *Middleware) methodCacheable(req *http.Request) bool {
	for _, m := range m.allowedMethods {
		if strings.EqualFold(m, req.Method) {
			return true
		}
	}
	return false
}
