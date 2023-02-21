package middleware

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/go-pkgz/requester/middleware/mocks"
)

func TestCircuitBreaker(t *testing.T) {

	cbMock := &mocks.CircuitBreakerSvcMock{
		ExecuteFunc: func(req func() (interface{}, error)) (interface{}, error) {
			return req()
		},
	}

	rmock := &mocks.RoundTripper{
		RoundTripFunc: func(r *http.Request) (*http.Response, error) {
			resp := &http.Response{StatusCode: 201}
			return resp, nil
		},
	}

	req, err := http.NewRequest("GET", "http://example.com/blah", http.NoBody)
	require.NoError(t, err)

	h := CircuitBreaker(cbMock)

	resp, err := h(rmock).RoundTrip(req)
	require.NoError(t, err)
	assert.Equal(t, 201, resp.StatusCode)

	assert.Equal(t, 1, rmock.Calls())
	assert.Equal(t, 1, len(cbMock.ExecuteCalls()))
}
