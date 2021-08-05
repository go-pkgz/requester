package requester

import (
	"bytes"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-pkgz/requester/middleware"
	"github.com/go-pkgz/requester/middleware/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

	req, err := http.NewRequest("GET", ts.URL, nil)
	require.NoError(t, err)

	resp, err := rq.Do(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	body, err := ioutil.ReadAll(resp.Body)
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

	req, err := http.NewRequest("GET", ts.URL, nil)
	require.NoError(t, err)

	resp, err := rq.Do(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	body, err := ioutil.ReadAll(resp.Body)
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
	req, err := http.NewRequest("GET", ts.URL, nil)
	require.NoError(t, err)
	resp, err := rq.Do(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	rq2 := rq.With(mw2)
	req, err = http.NewRequest("GET", ts.URL, nil)
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
	body, err := ioutil.ReadAll(resp.Body)
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
		body, err := ioutil.ReadAll(r.Body)
		assert.NoError(t, err)
		assert.Contains(t, string(body), "request body")
		_, err = w.Write([]byte("something"))
		require.NoError(t, err)
		time.Sleep(time.Duration(rand.Intn(10)) * time.Millisecond)
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

	req, err := http.NewRequest("GET", remoteTS.URL, nil)
	require.NoError(t, err)
	resp, err := rq.Do(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Greater(t, atomic.LoadInt32(&caughtReq), int32(0))

	req, err = http.NewRequest("GET", remoteTS.URL, nil)
	require.NoError(t, err)
	resp, err = rq.Client().Do(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Greater(t, atomic.LoadInt32(&caughtReq), int32(1))
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
	req, err := http.NewRequest("GET", "http://example.com", nil)
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
	req, err := http.NewRequest("GET", "http://example.com", nil)
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
