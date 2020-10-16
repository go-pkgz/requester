package middleware

import (
	"net/http"
	"testing"

	"github.com/go-pkgz/requester/middleware/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHeader(t *testing.T) {
	rmock := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
		assert.Equal(t, "v1", r.Header.Get("k1"))
		resp := &http.Response{StatusCode: 201}
		return resp, nil
	}}

	req, err := http.NewRequest("GET", "http://example.com/blah", nil)
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

	req, err := http.NewRequest("GET", "http://example.com/blah", nil)
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

	req, err := http.NewRequest("GET", "http://example.com/blah", nil)
	require.NoError(t, err)

	resp, err := BasicAuth("user", "passwd")(rmock).RoundTrip(req)
	require.NoError(t, err)
	assert.Equal(t, 201, resp.StatusCode)
	assert.Equal(t, 1, rmock.Calls())
}
