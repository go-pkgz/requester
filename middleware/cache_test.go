package middleware

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/go-pkgz/requester/middleware/mocks"
)

func TestCache_BasicCaching(t *testing.T) {
	t.Run("caches GET request", func(t *testing.T) {
		var requestCount int32
		rmock := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
			atomic.AddInt32(&requestCount, 1)
			return &http.Response{
				StatusCode: 200,
				Header:     http.Header{"X-Test": []string{"value"}},
				Body:       io.NopCloser(strings.NewReader("response body")),
			}, nil
		}}

		h := Cache()(rmock)
		req, err := http.NewRequest(http.MethodGet, "http://example.com/", http.NoBody)
		require.NoError(t, err)

		// first request - cache miss
		resp1, err := h.RoundTrip(req)
		require.NoError(t, err)
		body1, err := io.ReadAll(resp1.Body)
		require.NoError(t, err)
		_ = resp1.Body.Close()

		// second request - should be cached
		resp2, err := h.RoundTrip(req)
		require.NoError(t, err)
		body2, err := io.ReadAll(resp2.Body)
		require.NoError(t, err)
		_ = resp2.Body.Close()

		assert.Equal(t, int32(1), atomic.LoadInt32(&requestCount))
		assert.Equal(t, 200, resp1.StatusCode)
		assert.Equal(t, 200, resp2.StatusCode)
		assert.Equal(t, "response body", string(body1))
		assert.Equal(t, "response body", string(body2))
		assert.Equal(t, "value", resp1.Header.Get("X-Test"))
		assert.Equal(t, "value", resp2.Header.Get("X-Test"))
	})

	t.Run("does not cache POST by default", func(t *testing.T) {
		var requestCount int32
		rmock := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
			atomic.AddInt32(&requestCount, 1)
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader("response body")),
			}, nil
		}}

		h := Cache()(rmock)
		req, err := http.NewRequest(http.MethodPost, "http://example.com/", http.NoBody)
		require.NoError(t, err)

		// make two requests
		_, err = h.RoundTrip(req)
		require.NoError(t, err)
		_, err = h.RoundTrip(req)
		require.NoError(t, err)

		assert.Equal(t, int32(2), atomic.LoadInt32(&requestCount))
	})

	t.Run("does not cache non-200 by default", func(t *testing.T) {
		var requestCount int32
		rmock := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
			atomic.AddInt32(&requestCount, 1)
			return &http.Response{
				StatusCode: 404,
				Body:       io.NopCloser(strings.NewReader("not found")),
			}, nil
		}}

		h := Cache()(rmock)
		req, err := http.NewRequest(http.MethodGet, "http://example.com/", http.NoBody)
		require.NoError(t, err)

		// make two requests
		_, err = h.RoundTrip(req)
		require.NoError(t, err)
		_, err = h.RoundTrip(req)
		require.NoError(t, err)

		assert.Equal(t, int32(2), atomic.LoadInt32(&requestCount))
	})
}

func TestCache_Options(t *testing.T) {

	t.Run("respects TTL", func(t *testing.T) {
		var requestCount int32
		rmock := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
			atomic.AddInt32(&requestCount, 1)
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader("response body")),
			}, nil
		}}

		h := Cache(CacheTTL(50 * time.Millisecond))(rmock)
		req, err := http.NewRequest(http.MethodGet, "http://example.com/", http.NoBody)
		require.NoError(t, err)

		// first request
		_, err = h.RoundTrip(req)
		require.NoError(t, err)

		// second request - should be cached
		_, err = h.RoundTrip(req)
		require.NoError(t, err)
		assert.Equal(t, int32(1), atomic.LoadInt32(&requestCount))

		// wait for TTL to expire
		time.Sleep(100 * time.Millisecond)

		// third request - should hit the backend
		_, err = h.RoundTrip(req)
		require.NoError(t, err)
		assert.Equal(t, int32(2), atomic.LoadInt32(&requestCount))
	})

	t.Run("respects cache size", func(t *testing.T) {
		var requestCount int32
		rmock := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 200,
				Header:     http.Header{"X-Test": []string{"value"}},
				Body:       io.NopCloser(strings.NewReader(fmt.Sprintf("response %d", atomic.AddInt32(&requestCount, 1)))),
			}, nil
		}}

		h := Cache(CacheSize(2))(rmock)

		// first request - should be cached
		req1, _ := http.NewRequest(http.MethodGet, "http://example.com/1", http.NoBody)
		_, _ = h.RoundTrip(req1) // first call: should hit the backend
		_, _ = h.RoundTrip(req1) // second call: should be served from cache

		// second request - should be cached
		req2, _ := http.NewRequest(http.MethodGet, "http://example.com/2", http.NoBody)
		_, _ = h.RoundTrip(req2) // first call: should hit the backend
		_, _ = h.RoundTrip(req2) // second call: should be served from cache

		// third request - triggers eviction of first request
		req3, _ := http.NewRequest(http.MethodGet, "http://example.com/3", http.NoBody)
		_, _ = h.RoundTrip(req3)

		// first request should be evicted, making a new backend call
		_, _ = h.RoundTrip(req1)

		assert.Equal(t, int32(4), atomic.LoadInt32(&requestCount), "First request should be evicted and re-fetched")
	})

	t.Run("respects allowed methods", func(t *testing.T) {
		var requestCount int32
		rmock := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
			atomic.AddInt32(&requestCount, 1)
			return &http.Response{
				StatusCode: 200,
				Header:     http.Header{"X-Test": []string{"value"}},
				Body:       io.NopCloser(strings.NewReader("response body")),
			}, nil
		}}

		h := Cache(CacheMethods(http.MethodGet, http.MethodPost))(rmock)

		// GET request should be cached
		req1, _ := http.NewRequest(http.MethodGet, "http://example.com/", http.NoBody)
		_, err := h.RoundTrip(req1)
		require.NoError(t, err)
		_, err = h.RoundTrip(req1)
		require.NoError(t, err)
		assert.Equal(t, int32(1), atomic.LoadInt32(&requestCount), "GET requests should be cached")

		// POST request should be cached
		req2, _ := http.NewRequest(http.MethodPost, "http://example.com/", http.NoBody)
		_, err = h.RoundTrip(req2)
		require.NoError(t, err)
		_, err = h.RoundTrip(req2)
		require.NoError(t, err)
		assert.Equal(t, int32(2), atomic.LoadInt32(&requestCount), "POST requests should use different cache key")

		// PUT request should not be cached
		req3, _ := http.NewRequest(http.MethodPut, "http://example.com/", http.NoBody)
		_, err = h.RoundTrip(req3)
		require.NoError(t, err)
		_, err = h.RoundTrip(req3)
		require.NoError(t, err)
		assert.Equal(t, int32(4), atomic.LoadInt32(&requestCount), "PUT requests should not be cached")
	})
}

func TestCache_Keys(t *testing.T) {
	t.Run("different URLs get different cache entries", func(t *testing.T) {
		var requestCount int32
		rmock := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
			atomic.AddInt32(&requestCount, 1)
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(r.URL.Path)),
			}, nil
		}}

		h := Cache()(rmock)

		req1, err := http.NewRequest(http.MethodGet, "http://example.com/1", http.NoBody)
		require.NoError(t, err)
		resp1, err := h.RoundTrip(req1)
		require.NoError(t, err)
		body1, err := io.ReadAll(resp1.Body)
		require.NoError(t, err)
		err = resp1.Body.Close()
		require.NoError(t, err)

		req2, err := http.NewRequest(http.MethodGet, "http://example.com/2", http.NoBody)
		require.NoError(t, err)
		resp2, err := h.RoundTrip(req2)
		require.NoError(t, err)
		body2, err := io.ReadAll(resp2.Body)
		require.NoError(t, err)
		err = resp2.Body.Close()
		require.NoError(t, err)

		assert.Equal(t, "/1", string(body1))
		assert.Equal(t, "/2", string(body2))
		assert.Equal(t, int32(2), atomic.LoadInt32(&requestCount))
	})

	t.Run("includes headers in cache key when configured", func(t *testing.T) {
		var requestCount int32
		rmock := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
			atomic.AddInt32(&requestCount, 1)
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(r.Header.Get("X-Test"))),
			}, nil
		}}

		h := Cache(CacheWithHeaders("X-Test"))(rmock)
		req1, err := http.NewRequest(http.MethodGet, "http://example.com/", http.NoBody)
		require.NoError(t, err)
		req1.Header.Set("X-Test", "value1")
		resp1, err := h.RoundTrip(req1)
		require.NoError(t, err)
		body1, err := io.ReadAll(resp1.Body)
		require.NoError(t, err)
		err = resp1.Body.Close()
		require.NoError(t, err)

		req2, err := http.NewRequest(http.MethodGet, "http://example.com/", http.NoBody)
		require.NoError(t, err)
		req2.Header.Set("X-Test", "value2")
		resp2, err := h.RoundTrip(req2)
		require.NoError(t, err)
		body2, err := io.ReadAll(resp2.Body)
		require.NoError(t, err)
		err = resp2.Body.Close()
		require.NoError(t, err)

		assert.Equal(t, "value1", string(body1))
		assert.Equal(t, "value2", string(body2))
		assert.Equal(t, int32(2), atomic.LoadInt32(&requestCount))
	})

	t.Run("includes body in cache key when configured", func(t *testing.T) {
		var requestCount int32
		rmock := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
			atomic.AddInt32(&requestCount, 1)
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewReader(body)),
			}, nil
		}}

		h := Cache(CacheWithBody)(rmock)
		req1, err := http.NewRequest(http.MethodGet, "http://example.com/", strings.NewReader("body1"))
		require.NoError(t, err)
		resp1, err := h.RoundTrip(req1)
		require.NoError(t, err)
		body1, err := io.ReadAll(resp1.Body)
		require.NoError(t, err)
		err = resp1.Body.Close()
		require.NoError(t, err)

		req2, err := http.NewRequest(http.MethodGet, "http://example.com/", strings.NewReader("body2"))
		require.NoError(t, err)
		resp2, err := h.RoundTrip(req2)
		require.NoError(t, err)
		body2, err := io.ReadAll(resp2.Body)
		require.NoError(t, err)
		err = resp2.Body.Close()
		require.NoError(t, err)

		assert.Equal(t, "body1", string(body1))
		assert.Equal(t, "body2", string(body2))
		assert.Equal(t, int32(2), atomic.LoadInt32(&requestCount))
	})
}

func TestCache_EdgeCases(t *testing.T) {

	t.Run("expired cache entry should be ignored", func(t *testing.T) {
		var requestCount int32
		rmock := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
			atomic.AddInt32(&requestCount, 1)
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader("fresh response")),
			}, nil
		}}

		h := Cache(CacheTTL(50 * time.Millisecond))(rmock)

		req, err := http.NewRequest(http.MethodGet, "http://example.com/expired", http.NoBody)
		require.NoError(t, err, "failed to create request")

		_, err = h.RoundTrip(req)
		require.NoError(t, err, "first request should not fail")
		assert.Equal(t, int32(1), atomic.LoadInt32(&requestCount), "first request should hit backend")

		time.Sleep(100 * time.Millisecond)

		_, err = h.RoundTrip(req)
		require.NoError(t, err, "second request should not fail")
		assert.Equal(t, int32(2), atomic.LoadInt32(&requestCount), "expired cache entry should not be used")
	})

	t.Run("cache size 1 should evict immediately", func(t *testing.T) {
		var requestCount int32
		rmock := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
			atomic.AddInt32(&requestCount, 1)
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader("cached response")),
			}, nil
		}}

		h := Cache(CacheSize(1))(rmock)

		req1, err := http.NewRequest(http.MethodGet, "http://example.com/1", http.NoBody)
		require.NoError(t, err, "failed to create request 1")
		req2, err := http.NewRequest(http.MethodGet, "http://example.com/2", http.NoBody)
		require.NoError(t, err, "failed to create request 2")
		req3, err := http.NewRequest(http.MethodGet, "http://example.com/3", http.NoBody)
		require.NoError(t, err, "failed to create request 3")

		_, err = h.RoundTrip(req1)
		require.NoError(t, err)

		_, err = h.RoundTrip(req2)
		require.NoError(t, err)

		_, err = h.RoundTrip(req3)
		require.NoError(t, err)

		_, err = h.RoundTrip(req1)
		require.NoError(t, err)

		assert.Equal(t, int32(4), atomic.LoadInt32(&requestCount), "each request should evict the previous one")
	})

	t.Run("only specified status codes should be cached", func(t *testing.T) {
		var requestCount int32
		rmock := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
			atomic.AddInt32(&requestCount, 1)
			return &http.Response{
				StatusCode: 202, // not in allowed list
				Body:       io.NopCloser(strings.NewReader("not cached")),
			}, nil
		}}

		h := Cache(CacheStatuses(200, 201, 204))(rmock)

		req, err := http.NewRequest(http.MethodGet, "http://example.com/status", http.NoBody)
		require.NoError(t, err, "failed to create request")

		_, err = h.RoundTrip(req)
		require.NoError(t, err)
		assert.Equal(t, int32(1), atomic.LoadInt32(&requestCount), "first request should hit backend")

		_, err = h.RoundTrip(req)
		require.NoError(t, err)
		assert.Equal(t, int32(2), atomic.LoadInt32(&requestCount), "non-allowed status codes should not be cached")
	})

	t.Run("headers should be included in cache key when configured", func(t *testing.T) {
		var requestCount int32
		rmock := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
			atomic.AddInt32(&requestCount, 1)
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(r.Header.Get("Authorization"))),
			}, nil
		}}

		h := Cache(CacheWithHeaders("Authorization"))(rmock)

		req1, err := http.NewRequest(http.MethodGet, "http://example.com/auth", http.NoBody)
		require.NoError(t, err, "failed to create request 1")
		req1.Header.Set("Authorization", "Bearer token1")

		resp1, err := h.RoundTrip(req1)
		require.NoError(t, err)
		body1, err := io.ReadAll(resp1.Body)
		require.NoError(t, err)
		err = resp1.Body.Close()
		require.NoError(t, err)

		resp2, err := h.RoundTrip(req1) // second call should hit cache
		require.NoError(t, err)
		body2, err := io.ReadAll(resp2.Body)
		require.NoError(t, err)
		err = resp2.Body.Close()
		require.NoError(t, err)

		req2, err := http.NewRequest(http.MethodGet, "http://example.com/auth", http.NoBody)
		require.NoError(t, err, "failed to create request 2")
		req2.Header.Set("Authorization", "Bearer token2")

		resp3, err := h.RoundTrip(req2)
		require.NoError(t, err)
		body3, err := io.ReadAll(resp3.Body)
		require.NoError(t, err)
		err = resp3.Body.Close()
		require.NoError(t, err)

		assert.Equal(t, "Bearer token1", string(body1), "first request should be cached separately")
		assert.Equal(t, "Bearer token1", string(body2), "second request should be served from cache")
		assert.Equal(t, "Bearer token2", string(body3), "third request should be a new cache entry")
		assert.Equal(t, int32(2), atomic.LoadInt32(&requestCount), "each authorization header should generate a new cache entry")
	})
}
