package middleware

import (
	"context"
	"errors"
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

func TestRetry_BasicBehavior(t *testing.T) {
	t.Run("retries on network error", func(t *testing.T) {
		var attemptCount int32
		rmock := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
			count := atomic.AddInt32(&attemptCount, 1)
			if count < 3 {
				return nil, errors.New("network error")
			}
			return &http.Response{StatusCode: 200}, nil
		}}

		h := Retry(3, time.Millisecond)(rmock)
		req, err := http.NewRequest("GET", "http://example.com/", http.NoBody)
		require.NoError(t, err)

		resp, err := h.RoundTrip(req)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
		assert.Equal(t, int32(3), atomic.LoadInt32(&attemptCount))
	})

	t.Run("retries on 5xx status by default", func(t *testing.T) {
		var attemptCount int32
		rmock := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
			count := atomic.AddInt32(&attemptCount, 1)
			if count < 3 {
				return &http.Response{StatusCode: 503}, nil
			}
			return &http.Response{StatusCode: 200}, nil
		}}

		h := Retry(3, time.Millisecond)(rmock)
		req, err := http.NewRequest("GET", "http://example.com/", http.NoBody)
		require.NoError(t, err)

		resp, err := h.RoundTrip(req)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
		assert.Equal(t, int32(3), atomic.LoadInt32(&attemptCount))
	})

	t.Run("no retry on 4xx by default", func(t *testing.T) {
		var attemptCount int32
		rmock := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
			atomic.AddInt32(&attemptCount, 1)
			return &http.Response{StatusCode: 404}, nil
		}}

		h := Retry(3, time.Millisecond)(rmock)
		req, err := http.NewRequest("GET", "http://example.com/", http.NoBody)
		require.NoError(t, err)

		resp, err := h.RoundTrip(req)
		require.NoError(t, err)
		assert.Equal(t, 404, resp.StatusCode)
		assert.Equal(t, int32(1), atomic.LoadInt32(&attemptCount))
	})

	t.Run("fails with error after max attempts", func(t *testing.T) {
		var attemptCount int32
		rmock := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
			atomic.AddInt32(&attemptCount, 1)
			return nil, errors.New("persistent network error")
		}}

		h := Retry(3, time.Millisecond)(rmock)
		req, err := http.NewRequest("GET", "http://example.com/", http.NoBody)
		require.NoError(t, err)

		resp, err := h.RoundTrip(req)
		assert.Nil(t, resp)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "retry: transport error after 3 attempts")
		assert.Equal(t, int32(3), atomic.LoadInt32(&attemptCount))
	})

	t.Run("respects request context cancellation", func(t *testing.T) {
		var attemptCount int32
		rmock := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
			atomic.AddInt32(&attemptCount, 1)
			return nil, errors.New("network failure")
		}}

		ctx, cancel := context.WithCancel(context.Background())
		req, err := http.NewRequestWithContext(ctx, "GET", "http://example.com/", http.NoBody)
		require.NoError(t, err)

		h := Retry(5, 50*time.Millisecond)(rmock)

		// Cancel request after first attempt
		time.AfterFunc(20*time.Millisecond, cancel)

		_, err = h.RoundTrip(req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "context canceled")
		assert.Equal(t, int32(1), atomic.LoadInt32(&attemptCount), "should stop retrying after context cancellation")
	})
}

func TestRetry_BackoffStrategies(t *testing.T) {
	tests := []struct {
		name        string
		backoff     BackoffType
		minDuration time.Duration
		maxDuration time.Duration
	}{
		{
			name:        "constant backoff",
			backoff:     BackoffConstant,
			minDuration: 3 * time.Millisecond, // 1ms * 3
			maxDuration: 5 * time.Millisecond, // some buffer for execution time
		},
		{
			name:        "linear backoff",
			backoff:     BackoffLinear,
			minDuration: 6 * time.Millisecond, // 1ms + 2ms + 3ms
			maxDuration: 8 * time.Millisecond,
		},
		{
			name:        "exponential backoff",
			backoff:     BackoffExponential,
			minDuration: 7 * time.Millisecond, // 1ms + 2ms + 4ms
			maxDuration: 9 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var attemptCount int32
			rmock := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
				count := atomic.AddInt32(&attemptCount, 1)
				if count < 4 {
					return &http.Response{StatusCode: 503}, nil
				}
				return &http.Response{StatusCode: 200}, nil
			}}

			start := time.Now()
			h := Retry(4, time.Millisecond, RetryWithBackoff(tt.backoff))(rmock)
			req, err := http.NewRequest("GET", "http://example.com/", http.NoBody)
			require.NoError(t, err)

			resp, err := h.RoundTrip(req)
			duration := time.Since(start)

			require.NoError(t, err)
			assert.Equal(t, 200, resp.StatusCode)
			assert.Equal(t, int32(4), atomic.LoadInt32(&attemptCount))
			assert.GreaterOrEqual(t, duration, tt.minDuration)
			assert.LessOrEqual(t, duration, tt.maxDuration)
		})
	}

	t.Run("max delay limits backoff", func(t *testing.T) {
		var attemptCount int32
		rmock := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
			atomic.AddInt32(&attemptCount, 1)
			return &http.Response{StatusCode: 503}, nil
		}}

		start := time.Now()
		h := Retry(3, 10*time.Millisecond,
			RetryMaxDelay(15*time.Millisecond),
			RetryWithBackoff(BackoffExponential),
		)(rmock)
		req, err := http.NewRequest("GET", "http://example.com/", http.NoBody)
		require.NoError(t, err)

		_, _ = h.RoundTrip(req)
		duration := time.Since(start)

		// With exponential backoff and 10ms initial delay, without max delay
		// it would be 10ms + 20ms + 40ms = 70ms, but with max delay of 15ms
		// it should be 10ms + 15ms + 15ms = 40ms
		assert.Less(t, duration, 50*time.Millisecond)
	})

	t.Run("jitter factor affects delay", func(t *testing.T) {
		var callTimes []time.Time
		rmock := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
			callTimes = append(callTimes, time.Now())
			return &http.Response{StatusCode: 503}, nil
		}}

		h := Retry(3, 10*time.Millisecond,
			RetryWithJitter(0.5),
			RetryWithBackoff(BackoffConstant),
		)(rmock)
		req, err := http.NewRequest("GET", "http://example.com/", http.NoBody)
		require.NoError(t, err)

		_, _ = h.RoundTrip(req)

		require.Greater(t, len(callTimes), 2)
		delay1 := callTimes[1].Sub(callTimes[0])
		delay2 := callTimes[2].Sub(callTimes[1])
		// With 0.5 jitter and 10ms delay, delays should be different
		assert.NotEqual(t, delay1, delay2)
		// But both should be in range 5ms-15ms (10ms ±50%)
		assert.Greater(t, delay1, 5*time.Millisecond)
		assert.Less(t, delay1, 15*time.Millisecond)
		assert.Greater(t, delay2, 5*time.Millisecond)
		assert.Less(t, delay2, 15*time.Millisecond)
	})

	t.Run("verifies retry actually introduces delay", func(t *testing.T) {
		var attemptCount int32
		rmock := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
			count := atomic.AddInt32(&attemptCount, 1)
			if count < 4 {
				return &http.Response{StatusCode: 503}, nil
			}
			return &http.Response{StatusCode: 200}, nil
		}}

		start := time.Now()
		h := Retry(4, 5*time.Millisecond, RetryWithBackoff(BackoffExponential))(rmock)
		req, err := http.NewRequest("GET", "http://example.com/", http.NoBody)
		require.NoError(t, err)

		resp, err := h.RoundTrip(req)
		duration := time.Since(start)

		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
		assert.Equal(t, int32(4), atomic.LoadInt32(&attemptCount))

		// expected delay: 5ms + 10ms + 20ms = 35ms (exponential backoff)
		expectedMin := 30 * time.Millisecond
		expectedMax := 40 * time.Millisecond

		assert.Greater(t, duration, expectedMin, "retries should introduce actual delay")
		assert.LessOrEqual(t, duration, expectedMax, "delay should not exceed expected range")
	})
}

func TestRetry_RequestBodyHandling(t *testing.T) {
	t.Run("POST request with body retries correctly", func(t *testing.T) {
		var attemptCount int32
		expectedBody := "test request body"

		rmock := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
			count := atomic.AddInt32(&attemptCount, 1)

			// read and verify the body content
			bodyBytes, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			assert.Equal(t, expectedBody, string(bodyBytes), "body should be available on attempt %d", count)

			if count < 3 {
				return &http.Response{StatusCode: 503}, nil
			}
			return &http.Response{StatusCode: 200}, nil
		}}

		h := Retry(3, time.Millisecond)(rmock)
		body := strings.NewReader(expectedBody)
		req, err := http.NewRequest("POST", "http://example.com/", body)
		require.NoError(t, err)

		// http.NewRequest automatically sets GetBody for common body types like strings.Reader
		// let's verify this
		assert.NotNil(t, req.GetBody, "GetBody should be automatically set by http.NewRequest for strings.Reader")

		resp, err := h.RoundTrip(req)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
		assert.Equal(t, int32(3), atomic.LoadInt32(&attemptCount))
	})

	t.Run("PUT request with GetBody function retries correctly", func(t *testing.T) {
		var attemptCount int32
		expectedBody := "test request body with GetBody"

		rmock := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
			count := atomic.AddInt32(&attemptCount, 1)

			// read and verify the body content
			bodyBytes, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			assert.Equal(t, expectedBody, string(bodyBytes), "body should be available on attempt %d", count)

			if count < 3 {
				return &http.Response{StatusCode: 502}, nil
			}
			return &http.Response{StatusCode: 200}, nil
		}}

		h := Retry(3, time.Millisecond)(rmock)
		req, err := http.NewRequest("PUT", "http://example.com/", strings.NewReader(expectedBody))
		require.NoError(t, err)

		// set GetBody function for proper retry handling
		req.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader(expectedBody)), nil
		}

		resp, err := h.RoundTrip(req)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
		assert.Equal(t, int32(3), atomic.LoadInt32(&attemptCount))
	})

	t.Run("request without GetBody doesn't retry by default", func(t *testing.T) {
		var attemptCount int32
		expectedBody := "test request body"

		customReader := io.NopCloser(strings.NewReader(expectedBody))

		rmock := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
			count := atomic.AddInt32(&attemptCount, 1)

			// read the body content
			bodyBytes, err := io.ReadAll(r.Body)
			require.NoError(t, err)

			if count == 1 {
				assert.Equal(t, expectedBody, string(bodyBytes))
			} else {
				// should not retry when buffering disabled
				t.Error("unexpected retry when buffering is disabled")
			}

			return &http.Response{StatusCode: 503}, nil
		}}

		// default - buffering disabled
		h := Retry(3, time.Millisecond)(rmock)
		req, err := http.NewRequest("POST", "http://example.com/", customReader)
		require.NoError(t, err)

		resp, err := h.RoundTrip(req)
		assert.NoError(t, err)
		assert.Equal(t, 503, resp.StatusCode)
		assert.Equal(t, int32(1), atomic.LoadInt32(&attemptCount), "should only attempt once when buffering disabled")
	})

	t.Run("request without GetBody buffers when enabled", func(t *testing.T) {
		var attemptCount int32
		expectedBody := "test request body"

		// custom reader that doesn't get GetBody automatically
		customReader := io.NopCloser(strings.NewReader(expectedBody))

		rmock := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
			count := atomic.AddInt32(&attemptCount, 1)

			// read the body content
			bodyBytes, err := io.ReadAll(r.Body)
			require.NoError(t, err)

			// with auto-buffering, body should be available on all attempts
			assert.Equal(t, expectedBody, string(bodyBytes), "body should be available on attempt %d", count)

			if count < 3 {
				return &http.Response{StatusCode: 503}, nil
			}
			return &http.Response{StatusCode: 200}, nil
		}}

		h := Retry(3, time.Millisecond, RetryBufferBodies(true))(rmock)
		req, err := http.NewRequest("POST", "http://example.com/", customReader)
		require.NoError(t, err)

		// verify GetBody is nil for custom reader
		assert.Nil(t, req.GetBody, "GetBody should be nil for custom io.ReadCloser")

		resp, err := h.RoundTrip(req)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
		assert.Equal(t, int32(3), atomic.LoadInt32(&attemptCount))
	})

	t.Run("request with disabled buffering doesn't retry", func(t *testing.T) {
		var attemptCount int32
		expectedBody := "test request body"

		// custom reader that doesn't get GetBody automatically
		customReader := io.NopCloser(strings.NewReader(expectedBody))

		rmock := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
			count := atomic.AddInt32(&attemptCount, 1)

			// read the body content
			bodyBytes, err := io.ReadAll(r.Body)
			require.NoError(t, err)

			if count == 1 {
				// first and only attempt should have the body
				assert.Equal(t, expectedBody, string(bodyBytes))
			} else {
				t.Error("unexpected retry when buffering is disabled")
			}

			return &http.Response{StatusCode: 503}, nil
		}}

		h := Retry(3, time.Millisecond, RetryBufferBodies(false))(rmock)
		req, err := http.NewRequest("POST", "http://example.com/", customReader)
		require.NoError(t, err)

		resp, err := h.RoundTrip(req)
		require.NoError(t, err)
		assert.Equal(t, 503, resp.StatusCode)
		assert.Equal(t, int32(1), atomic.LoadInt32(&attemptCount), "should only attempt once when buffering disabled")
	})

	t.Run("request body exceeding max buffer size fails when buffering enabled", func(t *testing.T) {
		var attemptCount int32
		largeBody := strings.Repeat("x", 513) // just over 512 bytes

		customReader := io.NopCloser(strings.NewReader(largeBody))

		rmock := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
			atomic.AddInt32(&attemptCount, 1)
			return &http.Response{StatusCode: 503}, nil
		}}

		// enable buffering with max size of 512 bytes
		h := Retry(3, time.Millisecond, RetryBufferBodies(true), RetryMaxBufferSize(512))(rmock)
		req, err := http.NewRequest("POST", "http://example.com/", customReader)
		require.NoError(t, err)

		_, err = h.RoundTrip(req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "request body too large")
		assert.Contains(t, err.Error(), "513 bytes exceeds 512 byte limit")
		assert.Equal(t, int32(0), atomic.LoadInt32(&attemptCount), "should not attempt request when body too large")
	})

	t.Run("request body at max buffer size succeeds", func(t *testing.T) {
		var attemptCount int32
		bodyContent := strings.Repeat("x", 512) // exactly 512 bytes

		customReader := io.NopCloser(strings.NewReader(bodyContent))

		rmock := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
			count := atomic.AddInt32(&attemptCount, 1)

			// verify body is available on all attempts
			bodyBytes, err := io.ReadAll(r.Body)
			require.NoError(t, err)
			assert.Equal(t, bodyContent, string(bodyBytes))

			if count < 2 {
				return &http.Response{StatusCode: 503}, nil
			}
			return &http.Response{StatusCode: 200}, nil
		}}

		h := Retry(3, time.Millisecond, RetryBufferBodies(true), RetryMaxBufferSize(512))(rmock)
		req, err := http.NewRequest("POST", "http://example.com/", customReader)
		require.NoError(t, err)

		resp, err := h.RoundTrip(req)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
		assert.Equal(t, int32(2), atomic.LoadInt32(&attemptCount))
	})
}

func TestRetry_RetryConditions(t *testing.T) {
	t.Run("retry specific codes", func(t *testing.T) {
		var attemptCount int32
		rmock := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
			count := atomic.AddInt32(&attemptCount, 1)
			if count < 3 {
				return &http.Response{StatusCode: 418}, nil // teapot error
			}
			return &http.Response{StatusCode: 200}, nil
		}}

		h := Retry(3, time.Millisecond, RetryOnCodes(418))(rmock)
		req, err := http.NewRequest("GET", "http://example.com/", http.NoBody)
		require.NoError(t, err)

		resp, err := h.RoundTrip(req)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
		assert.Equal(t, int32(3), atomic.LoadInt32(&attemptCount))
	})

	t.Run("exclude codes from retry", func(t *testing.T) {
		var attemptCount int32
		rmock := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
			count := atomic.AddInt32(&attemptCount, 1)
			if count < 3 {
				return &http.Response{StatusCode: 404}, nil
			}
			return &http.Response{StatusCode: 200}, nil
		}}

		h := Retry(3, time.Millisecond, RetryExcludeCodes(503, 404))(rmock)
		req, err := http.NewRequest("GET", "http://example.com/", http.NoBody)
		require.NoError(t, err)

		resp, err := h.RoundTrip(req)
		require.NoError(t, err)
		assert.Equal(t, 404, resp.StatusCode)
		assert.Equal(t, int32(1), atomic.LoadInt32(&attemptCount))
	})

	t.Run("cannot use both include and exclude codes", func(t *testing.T) {
		assert.Panics(t, func() {
			rmock := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200}, nil
			}}
			_ = Retry(3, time.Millisecond,
				RetryOnCodes(503),
				RetryExcludeCodes(404),
			)(rmock)
		})
	})
}
