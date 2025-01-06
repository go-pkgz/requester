package middleware

import (
	"bytes"
	"context"
	"errors"
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

func TestRepeater_Passed(t *testing.T) {
	var count int32
	rmock := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
		if atomic.AddInt32(&count, 1) >= 3 {
			resp := &http.Response{StatusCode: 201}
			return resp, nil
		}
		resp := &http.Response{StatusCode: 400, Status: "400 Bad Request"}
		return resp, errors.New("blah")
	}}

	repeater := &mocks.RepeaterSvcMock{DoFunc: func(ctx context.Context, fun func() error, errs ...error) (err error) {
		for i := 0; i < 5; i++ {
			if err = fun(); err == nil {
				return nil
			}
		}
		return err
	}}

	h := Repeater(repeater)

	req, err := http.NewRequest("GET", "http://example.com/blah", http.NoBody)
	require.NoError(t, err)

	resp, err := h(rmock).RoundTrip(req)
	require.NoError(t, err)
	assert.Equal(t, 201, resp.StatusCode)

	assert.Equal(t, 3, rmock.Calls())
}

func TestRepeater_Failed(t *testing.T) {
	rmock := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
		resp := &http.Response{StatusCode: 400}
		return resp, errors.New("http error")
	}}

	repeater := &mocks.RepeaterSvcMock{DoFunc: func(ctx context.Context, fun func() error, errs ...error) (err error) {
		for i := 0; i < 5; i++ {
			if err = fun(); err == nil {
				return nil
			}
		}
		return err
	}}

	h := Repeater(repeater)

	req, err := http.NewRequest("GET", "http://example.com/blah", http.NoBody)
	require.NoError(t, err)

	_, err = h(rmock).RoundTrip(req)
	require.EqualError(t, err, "repeater: http error")

	assert.Equal(t, 5, rmock.Calls())
}

func TestRepeater_FailedStatus(t *testing.T) {
	rmock := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
		resp := &http.Response{StatusCode: 400, Status: "400 Bad Request"}
		return resp, nil
	}}

	repeater := &mocks.RepeaterSvcMock{DoFunc: func(ctx context.Context, fun func() error, errs ...error) (err error) {
		for i := 0; i < 5; i++ {
			if err = fun(); err == nil {
				return nil
			}
		}
		return err
	}}
	t.Run("no codes", func(t *testing.T) {
		rmock.ResetCalls()
		h := Repeater(repeater)
		req, err := http.NewRequest("GET", "http://example.com/blah", http.NoBody)
		require.NoError(t, err)

		_, err = h(rmock).RoundTrip(req)
		require.EqualError(t, err, "repeater: 400 Bad Request")
		assert.Equal(t, 5, rmock.Calls())
	})

	t.Run("with codes", func(t *testing.T) {
		rmock.ResetCalls()
		h := Repeater(repeater, 300, 400, 401)
		req, err := http.NewRequest("GET", "http://example.com/blah", http.NoBody)
		require.NoError(t, err)

		_, err = h(rmock).RoundTrip(req)
		require.EqualError(t, err, "repeater: 400 Bad Request")
		assert.Equal(t, 5, rmock.Calls())
	})

	t.Run("no codes, no match", func(t *testing.T) {
		rmock.ResetCalls()
		h := Repeater(repeater, 300, 401)

		req, err := http.NewRequest("GET", "http://example.com/blah", http.NoBody)
		require.NoError(t, err)

		resp, err := h(rmock).RoundTrip(req)
		require.NoError(t, err)
		assert.Equal(t, 400, resp.StatusCode)
		assert.Equal(t, 1, rmock.Calls())
	})
}

func TestRepeater_EdgeCases(t *testing.T) {

	t.Run("context cancellation", func(t *testing.T) {
		rmock := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
			time.Sleep(50 * time.Millisecond)
			return &http.Response{StatusCode: 500}, nil
		}}

		repeater := &mocks.RepeaterSvcMock{DoFunc: func(ctx context.Context, fun func() error, errs ...error) error {
			for i := 0; i < 5; i++ {
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
					if err := fun(); err == nil {
						return nil
					}
					time.Sleep(10 * time.Millisecond)
				}
			}
			return errors.New("max retries exceeded")
		}}

		h := Repeater(repeater)

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, "GET", "http://example.com/blah", http.NoBody)
		require.NoError(t, err)

		_, err = h(rmock).RoundTrip(req)
		require.Error(t, err)
		assert.True(t, errors.Is(err, context.DeadlineExceeded) ||
			strings.Contains(err.Error(), "context deadline exceeded"))
	})

	t.Run("retry with request body", func(t *testing.T) {
		var bodies []string
		rmock := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
			if r.Body == nil {
				bodies = append(bodies, "")
				return &http.Response{StatusCode: 500}, nil
			}
			body, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			bodies = append(bodies, string(body))
			// recreate body for next read
			r.Body = io.NopCloser(bytes.NewReader(body))
			return &http.Response{StatusCode: 500}, nil
		}}

		repeater := &mocks.RepeaterSvcMock{DoFunc: func(ctx context.Context, fun func() error, errs ...error) error {
			for i := 0; i < 3; i++ {
				if err := fun(); err == nil {
					return nil
				}
			}
			return errors.New("max retries")
		}}

		h := Repeater(repeater)

		bodyContent := "test body"
		req, err := http.NewRequest("POST", "http://example.com/blah",
			bytes.NewBufferString(bodyContent))
		require.NoError(t, err)

		_, err = h(rmock).RoundTrip(req)
		require.Error(t, err)
		assert.Equal(t, 3, len(bodies))
		for _, body := range bodies {
			assert.Equal(t, bodyContent, body)
		}
	})

	t.Run("status code ranges", func(t *testing.T) {
		cases := []struct {
			code        int
			failOnCodes []int
			retryCount  int
			expectError bool
			description string
		}{
			{
				code: 200, failOnCodes: []int{},
				retryCount: 1, expectError: false,
				description: "success with no explicit codes",
			},
			{
				code: 404, failOnCodes: []int{},
				retryCount: 5, expectError: true,
				description: "4xx with default fail codes",
			},
			{
				code: 503, failOnCodes: []int{},
				retryCount: 5, expectError: true,
				description: "5xx with default fail codes",
			},
			{
				code: 404, failOnCodes: []int{503},
				retryCount: 1, expectError: false,
				description: "4xx not in explicit codes",
			},
			{
				code: 503, failOnCodes: []int{503},
				retryCount: 5, expectError: true,
				description: "5xx in explicit codes",
			},
		}

		for _, tc := range cases {
			t.Run(tc.description, func(t *testing.T) {
				retryCount := 0
				rmock := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
					retryCount++
					return &http.Response{
						StatusCode: tc.code,
						Status:     fmt.Sprintf("%d Status", tc.code),
					}, nil
				}}

				repeater := &mocks.RepeaterSvcMock{DoFunc: func(ctx context.Context, fun func() error, errs ...error) error {
					var lastErr error
					for i := 0; i < 5; i++ {
						var err error
						if err = fun(); err == nil {
							return nil
						}
						lastErr = err
					}
					return fmt.Errorf("repeater: %w", lastErr)
				}}

				h := Repeater(repeater, tc.failOnCodes...)
				req, err := http.NewRequest("GET", "http://example.com/blah", http.NoBody)
				require.NoError(t, err)

				resp, err := h(rmock).RoundTrip(req)

				if tc.expectError {
					require.Error(t, err)
					assert.Contains(t, err.Error(), fmt.Sprint(tc.code))
					assert.Equal(t, tc.retryCount, retryCount, "unexpected retry count")
				} else {
					require.NoError(t, err)
					require.NotNil(t, resp)
					assert.Equal(t, tc.code, resp.StatusCode)
					assert.Equal(t, tc.retryCount, retryCount, "unexpected retry count")
				}
			})
		}
	})
}
