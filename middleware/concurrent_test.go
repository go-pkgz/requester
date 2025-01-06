package middleware

import (
	"context"
	"errors"
	"math/rand"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/go-pkgz/requester/middleware/mocks"
)

func TestMaxConcurrentHandler(t *testing.T) {
	var concurrentCount int32
	rmock := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
		c := atomic.AddInt32(&concurrentCount, 1)
		t.Logf("concurrent: %d", c)
		assert.True(t, c <= 8, c)
		defer func() {
			atomic.AddInt32(&concurrentCount, -1)
		}()
		resp := &http.Response{StatusCode: 201}
		time.Sleep(time.Millisecond * time.Duration(rand.Intn(100))) // nolint
		return resp, nil
	}}

	req, err := http.NewRequest("GET", "http://example.com/blah", http.NoBody)
	require.NoError(t, err)

	h := MaxConcurrent(8)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := h(rmock).RoundTrip(req)
			require.NoError(t, err)
			assert.Equal(t, 201, resp.StatusCode)
		}()
	}
	wg.Wait()

	assert.Equal(t, 100, rmock.Calls())
}

func TestMaxConcurrent_Advanced(t *testing.T) {

	t.Run("context cancellation", func(t *testing.T) {
		var active int32
		rmock := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
			atomic.AddInt32(&active, 1)
			defer atomic.AddInt32(&active, -1)

			select {
			case <-r.Context().Done():
				return nil, r.Context().Err()
			case <-time.After(100 * time.Millisecond):
				return &http.Response{StatusCode: 200}, nil
			}
		}}

		h := MaxConcurrent(2)
		wrapped := h(rmock)

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		var wg sync.WaitGroup
		for i := 0; i < 5; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				req, _ := http.NewRequestWithContext(ctx, "GET", "http://example.com/blah", http.NoBody)
				_, err := wrapped.RoundTrip(req)
				assert.Error(t, err)
				assert.True(t, errors.Is(err, context.DeadlineExceeded))
			}()
		}
		wg.Wait()

		assert.LessOrEqual(t, atomic.LoadInt32(&active), int32(2))
	})

	t.Run("goroutine behavior", func(t *testing.T) {
		var (
			active    int32
			maxActive int32
			finished  int32
		)

		rmock := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
			defer func() {
				atomic.AddInt32(&active, -1)
				atomic.AddInt32(&finished, 1)
			}()

			for {
				current := atomic.LoadInt32(&active)
				stored := atomic.LoadInt32(&maxActive)
				if current > stored {
					atomic.CompareAndSwapInt32(&maxActive, stored, current)
				}
				if current <= stored {
					break
				}
			}

			time.Sleep(10 * time.Millisecond)
			return &http.Response{StatusCode: 200}, nil
		}}

		h := MaxConcurrent(3)
		wrapped := h(rmock)

		var wg sync.WaitGroup
		numRequests := 10

		// start goroutines but ensure they start roughly at the same time
		readyChannel := make(chan struct{})
		for i := 0; i < numRequests; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-readyChannel // wait for signal
				req, _ := http.NewRequest("GET", "http://example.com/blah", http.NoBody)
				_, err := wrapped.RoundTrip(req)
				require.NoError(t, err)
			}()
		}

		// signal all goroutines to start
		close(readyChannel)
		wg.Wait()

		assert.Equal(t, int32(3), atomic.LoadInt32(&maxActive), "max concurrent should be 3")
		assert.Equal(t, int32(numRequests), atomic.LoadInt32(&finished), "all requests should finish")
	})

	t.Run("stress test", func(t *testing.T) {
		if testing.Short() {
			t.Skip("skipping stress test in short mode")
		}

		var (
			active    int32
			maxActive int32
			errs      int32
		)

		rmock := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
			defer atomic.AddInt32(&active, -1)
			for {
				current := atomic.LoadInt32(&active)
				stored := atomic.LoadInt32(&maxActive)
				if current > stored {
					atomic.CompareAndSwapInt32(&maxActive, stored, current)
				}
				if current <= stored {
					break
				}
			}

			// random delay and random errors
			time.Sleep(time.Duration(rand.Intn(20)) * time.Millisecond) //nolint:gosec // no need for secure random here

			if rand.Float32() < 0.1 { //nolint:gosec // no need for secure random here
				// 10% error rate
				atomic.AddInt32(&errs, 1)
				return nil, errors.New("random error")
			}

			return &http.Response{StatusCode: 200}, nil
		}}

		h := MaxConcurrent(5)
		wrapped := h(rmock)

		var wg sync.WaitGroup
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				req, _ := http.NewRequest("GET", "http://example.com/blah", http.NoBody)
				_, _ = wrapped.RoundTrip(req)
			}()
		}
		wg.Wait()

		assert.LessOrEqual(t, atomic.LoadInt32(&maxActive), int32(5), "should never exceed max concurrent")
		t.Logf("errors encountered: %d", atomic.LoadInt32(&errs))
	})
}
