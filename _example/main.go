package main

import (
	"log"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-pkgz/lcw"
	"github.com/go-pkgz/requester"
	"github.com/go-pkgz/requester/middleware"
	"github.com/go-pkgz/requester/middleware/cache"
	"github.com/go-pkgz/requester/middleware/logger"
)

func main() {

	// start test server
	ts := startTestServer()
	defer ts.Close()

	requestWithHeaders(ts)
	requestWithLogging(ts)
	requestWithCache(ts)
	requestWithCustom(ts)
	requestWithLimitConcurrency(ts)
}

// requestWithHeaders shows how to use requester with middleware altering headers
func requestWithHeaders(ts *httptest.Server) {
	rq := requester.New(http.Client{Timeout: 3 * time.Second}, middleware.JSON) // make requester with JSON headers
	// add auth header, user agent and JSON headers
	rq.Use(
		middleware.Header("X-Auth", "very-secret-key"),
		middleware.Header("User-Agent", "test-requester"),
		middleware.BasicAuth("user", "password"),
		middleware.JSON,
	)

	// create http.Request
	req, err := http.NewRequest("GET", ts.URL+"/blah", nil)
	if err != nil {
		panic(err)
	}

	resp, err := rq.Do(req)
	if err != nil {
		panic(err)
	}
	log.Printf("status: %s", resp.Status)

	// alternativley get http.Client and use directly
	client := rq.Client()
	resp, err = client.Get(ts.URL + "/blah")
	if err != nil {
		panic(err)
	}
	log.Printf("status: %s", resp.Status)
}

func requestWithLogging(ts *httptest.Server) {
	rq := requester.New(http.Client{Timeout: 3 * time.Second}, middleware.JSON) // make requester with JSON headers
	// add auth header, user agent and JSON headers
	// logging added after X-Auth to elinamte leaking it to the logs
	rq.Use(
		middleware.Header("X-Auth", "very-secret-key"),
		logger.New(logger.Std, logger.Prefix("REST"), logger.WithHeaders).Middleware,
		middleware.Header("User-Agent", "test-requester"),
		middleware.JSON,
	)

	// create http.Request
	req, err := http.NewRequest("GET", ts.URL+"/blah", nil)
	if err != nil {
		panic(err)
	}

	resp, err := rq.Do(req)
	if err != nil {
		panic(err)
	}
	log.Printf("status: %s", resp.Status)
}

func requestWithCache(ts *httptest.Server) {

	cacheService, err := lcw.NewLruCache(lcw.MaxKeys(100)) // make LRU loading cache
	if err != nil {
		panic(err)
	}

	cmw := cache.New(cacheService, cache.Methods("GET", "POST")) // create cache middleware

	// make requester with caching middleware and logger
	rq := requester.New(http.Client{Timeout: 3 * time.Second},
		cmw.Middleware,
		logger.New(logger.Std, logger.Prefix("REST CACHED"), logger.WithHeaders).Middleware,
	)

	// create http.Request
	req, err := http.NewRequest("GET", ts.URL+"/blah", nil)
	if err != nil {
		panic(err)
	}

	resp, err := rq.Do(req)
	if err != nil {
		panic(err)
	}
	log.Printf("status1: %s", resp.Status)

	// make another call for cached resurce, will be fast
	req2, err := http.NewRequest("GET", ts.URL+"/blah", nil)
	if err != nil {
		panic(err)
	}
	resp, err = rq.Do(req2)
	if err != nil {
		panic(err)
	}
	log.Printf("status2: %s", resp.Status)
}

func requestWithCustom(ts *httptest.Server) {

	// custome middlewre removes header foo
	clearHeaders := func(next http.RoundTripper) http.RoundTripper {
		fn := func(r *http.Request) (*http.Response, error) {
			r.Header.Del("foo")
			return next.RoundTrip(r)
		}
		return middleware.RoundTripperFunc(fn)
	}

	// make requester with logger
	rq := requester.New(http.Client{Timeout: 3 * time.Second},
		logger.New(logger.Std, logger.Prefix("REST CUSTOM"), logger.WithHeaders).Middleware,
		middleware.JSON,
	)

	// create http.Request
	req, err := http.NewRequest("GET", ts.URL+"/blah", nil)
	if err != nil {
		panic(err)
	}
	req.Header.Set("foo", "bar") // add foo header

	resp, err := rq.With(clearHeaders).Do(req) // can be used inline
	if err != nil {
		panic(err)
	}
	log.Printf("status: %s", resp.Status)
}

var inFly int32

func requestWithLimitConcurrency(ts *httptest.Server) {
	// make requester with logger and max concurrency 4
	rq := requester.New(http.Client{Timeout: 3 * time.Second},
		logger.New(logger.Std, logger.Prefix("REST CUSTOM"), logger.WithHeaders).Middleware,
		middleware.MaxConcurrent(4),
	)

	client := rq.Client()

	var wg sync.WaitGroup
	wg.Add(32)
	for i := 0; i < 32; i++ {
		go func(i int) {
			defer wg.Done()
			client.Get(ts.URL + "/blah" + strconv.Itoa(i))
			log.Printf("completed: %d, in fly:%d", i, atomic.LoadInt32(&inFly))
		}(i)
	}
	wg.Wait()
}

func startTestServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := atomic.AddInt32(&inFly, 1)
		log.Printf("request: %+v (%d)", r, c)
		time.Sleep(time.Duration(rand.Intn(100)) * time.Millisecond) // simulate random network latency
		w.Header().Set("k1", "v1")
		w.Write([]byte("something"))
		atomic.AddInt32(&inFly, -1)
	}))
}
