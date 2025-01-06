package middleware

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/go-pkgz/requester/middleware/mocks"
)

func TestHeader(t *testing.T) {
	rmock := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
		assert.Equal(t, "v1", r.Header.Get("k1"))
		resp := &http.Response{StatusCode: 201}
		return resp, nil
	}}

	req, err := http.NewRequest("GET", "http://example.com/blah", http.NoBody)
	require.NoError(t, err)

	h := Header("k1", "v1")
	resp, err := h(rmock).RoundTrip(req)
	require.NoError(t, err)
	assert.Equal(t, 201, resp.StatusCode)

	assert.Equal(t, 1, rmock.Calls())
}

func TestJSON(t *testing.T) {
	rmock := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "application/json", r.Header.Get("Accept"))
		resp := &http.Response{StatusCode: 201}
		return resp, nil
	}}

	req, err := http.NewRequest("GET", "http://example.com/blah", http.NoBody)
	require.NoError(t, err)

	resp, err := JSON(rmock).RoundTrip(req)
	require.NoError(t, err)
	assert.Equal(t, 201, resp.StatusCode)
	assert.Equal(t, 1, rmock.Calls())
}

func TestBasicAuth(t *testing.T) {
	rmock := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
		assert.Equal(t, "Basic dXNlcjpwYXNzd2Q=", r.Header.Get("Authorization"))
		resp := &http.Response{StatusCode: 201}
		return resp, nil
	}}

	req, err := http.NewRequest("GET", "http://example.com/blah", http.NoBody)
	require.NoError(t, err)

	resp, err := BasicAuth("user", "passwd")(rmock).RoundTrip(req)
	require.NoError(t, err)
	assert.Equal(t, 201, resp.StatusCode)
	assert.Equal(t, 1, rmock.Calls())
}
func TestHeader_EdgeCases(t *testing.T) {
	t.Run("case insensitive", func(t *testing.T) {
		rmock := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
			assert.Equal(t, "v1", r.Header.Get("key"))
			assert.Equal(t, "v1", r.Header.Get("Key"))
			assert.Equal(t, "v1", r.Header.Get("KEY"))
			return &http.Response{StatusCode: 200}, nil
		}}

		req, err := http.NewRequest("GET", "http://example.com", http.NoBody)
		require.NoError(t, err)

		h := Header("KEY", "v1")
		_, err = h(rmock).RoundTrip(req)
		require.NoError(t, err)
	})

	t.Run("header overwrite", func(t *testing.T) {
		rmock := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
			// header middleware overwrites existing values
			assert.Equal(t, []string{"v2"}, r.Header.Values("key"))
			return &http.Response{StatusCode: 200}, nil
		}}

		req, err := http.NewRequest("GET", "http://example.com", http.NoBody)
		require.NoError(t, err)
		req.Header.Add("key", "v1")

		h := Header("key", "v2")
		_, err = h(rmock).RoundTrip(req)
		require.NoError(t, err)
	})

	t.Run("json headers set", func(t *testing.T) {
		rmock := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
			// JSON middleware sets both Content-Type and Accept
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
			assert.Equal(t, "application/json", r.Header.Get("Accept"))
			return &http.Response{StatusCode: 200}, nil
		}}

		req, err := http.NewRequest("GET", "http://example.com", http.NoBody)
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/xml")

		resp, err := JSON(rmock).RoundTrip(req)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
	})

	t.Run("basic auth headers", func(t *testing.T) {
		rmock := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
			user, pass, ok := r.BasicAuth()
			assert.True(t, ok)
			assert.Equal(t, "user", user)
			assert.Equal(t, "pass123$$!@", pass)
			return &http.Response{StatusCode: 200}, nil
		}}

		req, err := http.NewRequest("GET", "http://example.com", http.NoBody)
		require.NoError(t, err)

		h := BasicAuth("user", "pass123$$!@")
		resp, err := h(rmock).RoundTrip(req)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
	})

	t.Run("header middleware order", func(t *testing.T) {
		rmock := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
			assert.Equal(t, "v1", r.Header.Get("key1"))
			assert.Equal(t, "v2", r.Header.Get("key2"))
			// JSON middleware runs last in the chain, so it overwrites Content-Type
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
			user, pass, ok := r.BasicAuth()
			assert.True(t, ok)
			assert.Equal(t, "user", user)
			assert.Equal(t, "pass", pass)
			return &http.Response{StatusCode: 200}, nil
		}}

		req, err := http.NewRequest("GET", "http://example.com", http.NoBody)
		require.NoError(t, err)

		h1 := Header("key1", "v1")
		h2 := Header("key2", "v2")
		h3 := BasicAuth("user", "pass")
		h4 := JSON

		resp, err := h1(h2(h3(h4(rmock)))).RoundTrip(req)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
	})

	t.Run("empty headers", func(t *testing.T) {
		rmock := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
			assert.Equal(t, "", r.Header.Get("empty-key"))
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
			return &http.Response{StatusCode: 200}, nil
		}}

		req, err := http.NewRequest("GET", "http://example.com", http.NoBody)
		require.NoError(t, err)

		h := Header("empty-key", "")
		_, err = h(JSON(rmock)).RoundTrip(req)
		require.NoError(t, err)
	})
}
