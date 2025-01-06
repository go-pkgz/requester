package requester

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/go-pkgz/requester/middleware"
	"github.com/go-pkgz/requester/middleware/logger"
	"github.com/go-pkgz/requester/middleware/mocks"
)

func TestRequester_DoSimpleMiddleware(t *testing.T) {

	mw := func(next http.RoundTripper) http.RoundTripper {
		fn := func(req *http.Request) (*http.Response, error) {
			req.Header.Set("test", "blah")
			return next.RoundTrip(req)
		}
		return middleware.RoundTripperFunc(fn)
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		assert.Equal(t, "blah", r.Header.Get("test"))
		_, err := w.Write([]byte("something"))
		require.NoError(t, err)
	}))
	defer ts.Close()

	rq := New(http.Client{Timeout: 1 * time.Second}, mw)

	req, err := http.NewRequest("GET", ts.URL, http.NoBody)
	require.NoError(t, err)

	resp, err := rq.Do(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	assert.NoError(t, err)
	assert.Equal(t, "something", string(body))
}

func TestRequester_DoMiddlewareChain(t *testing.T) {
	mw1 := func(next http.RoundTripper) http.RoundTripper {
		fn := func(r *http.Request) (*http.Response, error) {
			r.Header.Set("test", "blah")
			return next.RoundTrip(r)
		}
		return middleware.RoundTripperFunc(fn)
	}
	mw2 := func(next http.RoundTripper) http.RoundTripper {
		fn := func(r *http.Request) (*http.Response, error) {
			r.Header.Set("test2", "blah2")
			return next.RoundTrip(r)
		}
		return middleware.RoundTripperFunc(fn)
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		assert.Equal(t, "blah", r.Header.Get("test"))
		assert.Equal(t, "blah2", r.Header.Get("test2"))
		_, err := w.Write([]byte("something"))
		require.NoError(t, err)
	}))
	defer ts.Close()

	rq := New(http.Client{Timeout: 1 * time.Second})
	rq.Use(mw1)
	rq.Use(mw2)

	req, err := http.NewRequest("GET", ts.URL, http.NoBody)
	require.NoError(t, err)

	resp, err := rq.Do(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	assert.NoError(t, err)
	assert.Equal(t, "something", string(body))
}

func TestRequester_With(t *testing.T) {

	mw1 := func(next http.RoundTripper) http.RoundTripper {
		fn := func(r *http.Request) (*http.Response, error) {
			r.Header.Set("test", "blah")
			return next.RoundTrip(r)
		}
		return middleware.RoundTripperFunc(fn)
	}
	mw2 := func(next http.RoundTripper) http.RoundTripper {
		fn := func(r *http.Request) (*http.Response, error) {
			r.Header.Set("test2", "blah2")
			return next.RoundTrip(r)
		}
		return middleware.RoundTripperFunc(fn)
	}

	var count int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		assert.Equal(t, "blah", r.Header.Get("test"))
		if atomic.LoadInt32(&count) == 0 {
			assert.Equal(t, "", r.Header.Get("test2"))
		}
		if atomic.LoadInt32(&count) == 1 {
			assert.Equal(t, "blah2", r.Header.Get("test2"))
		}
		_, err := w.Write([]byte("something"))
		require.NoError(t, err)
		atomic.AddInt32(&count, 1)
	}))
	defer ts.Close()

	rq := New(http.Client{Timeout: 1 * time.Second}, mw1)
	req, err := http.NewRequest("GET", ts.URL, http.NoBody)
	require.NoError(t, err)
	resp, err := rq.Do(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	rq2 := rq.With(mw2)
	req, err = http.NewRequest("GET", ts.URL, http.NoBody)
	require.NoError(t, err)
	resp, err = rq2.Do(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestRequester_Client(t *testing.T) {
	mw := func(next http.RoundTripper) http.RoundTripper {
		fn := func(req *http.Request) (*http.Response, error) {
			req.Header.Set("test", "blah")
			return next.RoundTrip(req)
		}
		return middleware.RoundTripperFunc(fn)
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		assert.Equal(t, "blah", r.Header.Get("test"))
		_, err := w.Write([]byte("something"))
		require.NoError(t, err)
	}))
	defer ts.Close()

	rq := New(http.Client{Timeout: 1 * time.Second}, mw)
	resp, err := rq.Client().Get(ts.URL)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	body, err := io.ReadAll(resp.Body)
	assert.NoError(t, err)
	assert.Equal(t, "something", string(body))
}

func TestRequester_CustomMiddleware(t *testing.T) {

	maskHeader := func(next http.RoundTripper) http.RoundTripper {
		fn := func(req *http.Request) (*http.Response, error) {
			req.Header.Del("deleteme")
			return next.RoundTrip(req)
		}
		return middleware.RoundTripperFunc(fn)
	}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "blah2", r.Header.Get("do-not-deleteme"))
		assert.Equal(t, "", r.Header.Get("deleteme"))
		body, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		assert.Contains(t, string(body), "request body")
		_, err = w.Write([]byte("something"))
		require.NoError(t, err)
		time.Sleep(time.Duration(rand.Intn(10)) * time.Millisecond) // nolint
	}))
	defer ts.Close()

	rqMasked := New(http.Client{}, logger.New(logger.Std, logger.WithHeaders).Middleware, middleware.JSON, maskHeader)
	req, err := http.NewRequest("POST", ts.URL, bytes.NewBufferString("request body"))
	require.NoError(t, err)
	req.Header.Set("deleteme", "blah1")
	req.Header.Set("do-not-deleteme", "blah2")
	resp, err := rqMasked.Do(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
}

func TestRequester_DoNotReplaceTransport(t *testing.T) {
	remoteTS := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatal("remote should not be reached due to redirect")
	}))
	defer remoteTS.Close()

	// indicates that the request was caught by the test server,
	// to which we are redirecting the request
	caughtReq := int32(0)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&caughtReq, 1)
		assert.Equal(t, "value", r.Header.Get("blah"))
		_, err := w.Write([]byte("something"))
		require.NoError(t, err)
	}))
	defer ts.Close()
	tsURL, err := url.Parse(ts.URL)
	require.NoError(t, err)

	redirectingRoundTripper := middleware.RoundTripperFunc(func(r *http.Request) (*http.Response, error) {
		r.URL = tsURL
		return http.DefaultTransport.RoundTrip(r)
	})

	rq := New(http.Client{Transport: redirectingRoundTripper}, middleware.Header("blah", "value"))

	req, err := http.NewRequest("GET", remoteTS.URL, http.NoBody)
	require.NoError(t, err)
	resp, err := rq.Do(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Greater(t, atomic.LoadInt32(&caughtReq), int32(0))

	req, err = http.NewRequest("GET", remoteTS.URL, http.NoBody)
	require.NoError(t, err)
	resp, err = rq.Client().Do(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Greater(t, atomic.LoadInt32(&caughtReq), int32(1))
}

func TestRequester_TransportHandling(t *testing.T) {
	const baseURL = "http://example.com"

	t.Run("custom transport preserved", func(t *testing.T) {
		customTransport := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200}, nil
		}}

		client := http.Client{Transport: customTransport}
		rq := New(client)

		req, err := http.NewRequest(http.MethodGet, baseURL, http.NoBody)
		require.NoError(t, err)
		resp, err := rq.Do(req)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
		assert.Equal(t, 1, customTransport.Calls())
	})

	t.Run("transport reused between calls", func(t *testing.T) {
		customTransport := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200}, nil
		}}

		client := http.Client{Transport: customTransport}
		rq := New(client)

		for i := 0; i < 3; i++ {
			req, err := http.NewRequest(http.MethodGet, baseURL, http.NoBody)
			require.NoError(t, err)
			resp, err := rq.Do(req)
			require.NoError(t, err)
			assert.Equal(t, 200, resp.StatusCode)
		}
		assert.Equal(t, 3, customTransport.Calls())
	})

	t.Run("nil transport uses default", func(t *testing.T) {
		client := http.Client{Transport: nil}
		rq := New(client)
		_, err := http.NewRequest(http.MethodGet, baseURL, http.NoBody)
		require.NoError(t, err)
		cl := rq.Client()
		assert.Equal(t, http.DefaultTransport, cl.Transport)
	})
}

func TestRequester_MiddlewareHandling(t *testing.T) {
	const baseURL = "http://example.com"

	t.Run("chaining with With", func(t *testing.T) {
		var calls []string
		mw1 := func(next http.RoundTripper) http.RoundTripper {
			return middleware.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
				calls = append(calls, "mw1")
				return next.RoundTrip(req)
			})
		}
		mw2 := func(next http.RoundTripper) http.RoundTripper {
			return middleware.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
				calls = append(calls, "mw2")
				return next.RoundTrip(req)
			})
		}

		transport := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200}, nil
		}}

		base := New(http.Client{Transport: transport}, mw1)
		r2 := base.With(mw2)

		req, err := http.NewRequest(http.MethodGet, baseURL, http.NoBody)
		require.NoError(t, err)
		_, err = r2.Do(req)
		require.NoError(t, err)
		assert.Equal(t, []string{"mw2", "mw1"}, calls)
	})

	t.Run("nil middleware allowed", func(t *testing.T) {
		transport := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200}, nil
		}}

		rq := New(http.Client{Transport: transport})
		rq.Use()       // empty Use call
		rq = rq.With() // empty With call

		req, err := http.NewRequest(http.MethodGet, baseURL, http.NoBody)
		require.NoError(t, err)
		resp, err := rq.Do(req)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
		assert.Equal(t, 1, transport.Calls())
	})

	t.Run("chains kept separate", func(t *testing.T) {
		var rq1Calls, rq2Calls []string
		mw1 := func(next http.RoundTripper) http.RoundTripper {
			return middleware.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
				rq1Calls = append(rq1Calls, "mw1")
				return next.RoundTrip(req)
			})
		}
		mw2 := func(next http.RoundTripper) http.RoundTripper {
			return middleware.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
				rq2Calls = append(rq2Calls, "mw2")
				return next.RoundTrip(req)
			})
		}

		transport := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200}, nil
		}}

		rq1 := New(http.Client{Transport: transport})
		rq2 := New(http.Client{Transport: transport})
		rq1.Use(mw1)
		rq2.Use(mw2)

		req, _ := http.NewRequest(http.MethodGet, baseURL, http.NoBody)
		_, _ = rq1.Do(req)
		_, _ = rq2.Do(req)

		assert.Equal(t, []string{"mw1"}, rq1Calls)
		assert.Equal(t, []string{"mw2"}, rq2Calls)
	})
}

func TestRequester_ErrorHandling(t *testing.T) {
	const baseURL = "http://example.com"

	t.Run("error from middleware", func(t *testing.T) {
		expectedErr := errors.New("custom error")
		errorMW := func(next http.RoundTripper) http.RoundTripper {
			return middleware.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
				return nil, expectedErr
			})
		}

		rq := New(http.Client{})
		rq.Use(errorMW)

		req, err := http.NewRequest(http.MethodGet, baseURL, http.NoBody)
		require.NoError(t, err)
		_, err = rq.Do(req)
		assert.ErrorIs(t, err, expectedErr)
	})

	t.Run("error propagation chain", func(t *testing.T) {
		var calls []string
		mw1 := func(next http.RoundTripper) http.RoundTripper {
			return middleware.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
				calls = append(calls, "mw1-before")
				resp, err := next.RoundTrip(req)
				if err != nil {
					calls = append(calls, "mw1-error")
				}
				return resp, err
			})
		}

		expectedErr := errors.New("transport error")
		transport := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
			return nil, expectedErr
		}}

		rq := New(http.Client{Transport: transport}, mw1)
		req, _ := http.NewRequest(http.MethodGet, baseURL, http.NoBody)
		_, err := rq.Do(req)
		require.Error(t, err)
		assert.Equal(t, []string{"mw1-before", "mw1-error"}, calls)
		assert.ErrorIs(t, err, expectedErr)
	})
}

func TestRequester_Timeouts(t *testing.T) {
	const baseURL = "http://example.com"

	t.Run("client timeout", func(t *testing.T) {
		transport := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
			select {
			case <-r.Context().Done():
				return nil, r.Context().Err()
			case <-time.After(100 * time.Millisecond):
				return &http.Response{StatusCode: 200}, nil
			}
		}}

		client := http.Client{
			Transport: transport,
			Timeout:   50 * time.Millisecond,
		}
		rq := New(client)

		req, err := http.NewRequest(http.MethodGet, baseURL, http.NoBody)
		require.NoError(t, err)
		_, err = rq.Do(req)
		require.Error(t, err)
		assert.True(t, strings.Contains(err.Error(), "context deadline exceeded") ||
			strings.Contains(err.Error(), "Client.Timeout"))
	})

	t.Run("request timeout", func(t *testing.T) {
		transport := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
			select {
			case <-r.Context().Done():
				return nil, r.Context().Err()
			case <-time.After(100 * time.Millisecond):
				return &http.Response{StatusCode: 200}, nil
			}
		}}

		rq := New(http.Client{Transport: transport})
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL, http.NoBody)
		require.NoError(t, err)
		_, err = rq.Do(req)
		require.Error(t, err)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	})
}

func ExampleNew() {
	// make requester, set JSON headers middleware
	rq := New(http.Client{Timeout: 3 * time.Second}, middleware.JSON)

	// add auth header, user agent and a custom X-Auth header middlewared
	rq.Use(
		middleware.Header("X-Auth", "very-secret-key"),
		middleware.Header("User-Agent", "test-requester"),
		middleware.BasicAuth("user", "password"),
	)
}

func ExampleRequester_Do() {
	rq := New(http.Client{Timeout: 3 * time.Second}) // make new requester

	// add logger, auth header, user agent and JSON headers
	rq.Use(
		middleware.Header("X-Auth", "very-secret-key"),
		logger.New(logger.Std, logger.Prefix("REST"), logger.WithHeaders).Middleware, // uses std logger
		middleware.Header("User-Agent", "test-requester"),
		middleware.JSON,
	)

	// create http.Request
	req, err := http.NewRequest("GET", "http://example.com", http.NoBody)
	if err != nil {
		panic(err)
	}

	// Send request and get reposnse
	resp, err := rq.Do(req)
	if err != nil {
		panic(err)
	}
	log.Printf("status: %s", resp.Status)
}

func ExampleRequester_Client() {
	// make new requester with some middlewares
	rq := New(http.Client{Timeout: 3 * time.Second},
		middleware.JSON,
		middleware.Header("User-Agent", "test-requester"),
		middleware.BasicAuth("user", "password"),
		middleware.MaxConcurrent(4),
	)

	client := rq.Client() // get http.Client
	resp, err := client.Get("http://example.com")
	if err != nil {
		panic(err)
	}
	log.Printf("status: %s", resp.Status)
}

func ExampleRequester_With() {
	rq1 := New(http.Client{Timeout: 3 * time.Second}, middleware.JSON) // make a requester with JSON middleware

	// make another requester inherited from rq1 with extra middlewares
	rq2 := rq1.With(middleware.BasicAuth("user", "password"), middleware.MaxConcurrent(4))

	// create http.Request
	req, err := http.NewRequest("GET", "http://example.com", http.NoBody)
	if err != nil {
		panic(err)
	}

	// send request with rq1 (JSON headers only)
	resp, err := rq1.Do(req)
	if err != nil {
		panic(err)
	}
	log.Printf("status1: %s", resp.Status)

	// send request with rq2 (JSON headers, basic auth and limiteted concurrecny)
	resp, err = rq2.Do(req)
	if err != nil {
		panic(err)
	}
	log.Printf("status2: %s", resp.Status)
}
