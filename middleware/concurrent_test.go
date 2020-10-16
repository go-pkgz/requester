package middleware

import (
	"math/rand"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-pkgz/requester/middleware/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		time.Sleep(time.Millisecond * time.Duration(rand.Intn(100)))
		return resp, nil
	}}

	req, err := http.NewRequest("GET", "http://example.com/blah", nil)
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
