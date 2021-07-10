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
	"github.com/go-pkgz/repeater"

	"github.com/go-pkgz/requester"
	"github.com/go-pkgz/requester/middleware"
	"github.com/go-pkgz/requester/middleware/cache"
	"github.com/go-pkgz/requester/middleware/logger"
)

func main() {

	// start test server
	ts := startTestServer()
	defer ts.Close()

	// start another test server returning 500 status on 5 first calls, for repeater demo
	tsRep := startTestServerFailedFirst()
	defer tsRep.Close()

	requestWithHeaders(ts)
	requestWithLogging(ts)
	requestWithCache(ts)
	requestWithCustom(ts)
	requestWithLimitConcurrency(ts)
	requestWithRepeater(tsRep)
}

// requestWithHeaders shows how to use requester with middleware altering headers
func requestWithHeaders(ts *httptest.Server) {
	log.Printf("requestWithHeaders --------------")
	rq := requester.New(http.Client{Timeout: 3 * time.Second}, middleware.JSON) // make requester with JSON headers
	// add auth header, user agent and basic auth
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

	// alternatively, get http.Client and use it directly
	client := rq.Client()
	resp, err = client.Get(ts.URL + "/blah")
	if err != nil {
		panic(err)
	}
	log.Printf("status: %s", resp.Status)
}

// requestWithLogging example of logging
func requestWithLogging(ts *httptest.Server) {
	log.Printf("requestWithLogging --------------")

	rq := requester.New(http.Client{Timeout: 3 * time.Second}, middleware.JSON) // make requester with JSON headers
	// add auth header, user agent and JSON headers
	// logging added after setting X-Auth to eliminate leaking it to the logs
	rq.Use(
		middleware.Header("X-Auth", "very-secret-key"),
		logger.New(logger.Std, logger.Prefix("REST"), logger.WithHeaders).Middleware, // uses std logger
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

// requestWithCache example of using request cache
func requestWithCache(ts *httptest.Server) {
	log.Printf("requestWithCache --------------")

	cacheService, err := lcw.NewLruCache(lcw.MaxKeys(100)) // make LRU loading cache
	if err != nil {
		panic(err)
	}

	// create cache middleware, allowing GET and POST responses caching
	// by default, caching key made from request's URL
	cmw := cache.New(cacheService, cache.Methods("GET", "POST"))

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

	resp, err := rq.Do(req) // the first call hits the remote endpoint and cache response
	if err != nil {
		panic(err)
	}
	log.Printf("status1: %s", resp.Status)

	// make another call for cached resource, will be fast as result cached
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

// requestWithCustom example of a custom, user provided middleware
func requestWithCustom(ts *httptest.Server) {
	log.Printf("requestWithCustom --------------")

	// custom middleware, removes header foo
	clearHeaders := func(next http.RoundTripper) http.RoundTripper {
		fn := func(r *http.Request) (*http.Response, error) {
			r.Header.Del("foo")
			return next.RoundTrip(r)
		}
		return middleware.RoundTripperFunc(fn)
	}

	// make requester with clearHeaders
	rq := requester.New(http.Client{Timeout: 3 * time.Second},
		logger.New(logger.Std, logger.Prefix("REST CUSTOM"), logger.WithHeaders).Middleware,
		clearHeaders,
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

// requestWithLimitConcurrency example of concurrency limiter
func requestWithLimitConcurrency(ts *httptest.Server) {
	log.Printf("requestWithLimitConcurrency --------------")

	// make requester with logger and max concurrency 4
	rq := requester.New(http.Client{Timeout: 3 * time.Second},
		logger.New(logger.Std, logger.Prefix("REST CUSTOM"), logger.WithHeaders).Middleware,
		middleware.MaxConcurrent(4),
	)

	client := rq.Client()

	// a test checking if concurrent requests limited to 4
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

// requestWithRepeater example of repeater usage
func requestWithRepeater(ts *httptest.Server) {
	log.Printf("requestWithRepeater --------------")

	rpt := repeater.NewDefault(10, 500*time.Millisecond) // make a repeater with up to 10 calls, 500ms between calls
	rq := requester.New(http.Client{},
		// repeat failed call up to 10 times with 500ms delay on networking error or given status codes
		middleware.Repeater(rpt, http.StatusInternalServerError, http.StatusBadGateway),
		logger.New(logger.Std, logger.Prefix("REST REPT"), logger.WithHeaders).Middleware,
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

func startTestServerFailedFirst() *httptest.Server {
	var n int32
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := atomic.AddInt32(&inFly, 1)
		log.Printf("request: %+v (%d)", r, c)
		time.Sleep(time.Duration(rand.Intn(100)) * time.Millisecond) // simulate random network latency

		if atomic.AddInt32(&n, 1) < 5 { // fail 5 first requests
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.Header().Set("k1", "v1")
		w.Write([]byte("something"))
		atomic.AddInt32(&inFly, -1)
	}))
}
