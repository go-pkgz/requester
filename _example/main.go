package main

import (
	"log"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/go-pkgz/requester"
	"github.com/go-pkgz/requester/middleware"
	"github.com/go-pkgz/requester/middleware/logger"
)

func main() {

	// start test server
	ts := startTestServer()
	defer ts.Close()

	rq := requester.New(http.Client{Timeout: 3 * time.Second}, middleware.JSON) // make requester with JSON headers
	// add auth header and user agent
	rq.Use(middleware.Header("X-Auth", "very-secret-key"), middleware.Header("User-Agent", "test-requester"))

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

	// create another http.Request
	req, err = http.NewRequest("POST", ts.URL+"/blah-blah", nil)
	if err != nil {
		panic(err)
	}

	// make requester with all rq middlewares plus with logger and mac concurrent
	lg := logger.New(logger.Std, logger.Prefix("DEBUG"), logger.WithHeaders)
	rq2 := rq.With(lg.Middleware, middleware.MaxConcurrent(4), middleware.Header("foo", "bar"))

	resp, err = rq2.Do(req)
	if err != nil {
		panic(err)
	}
	log.Printf("status: %s", resp.Status)

	// create another http.Request
	req, err = http.NewRequest("PUT", ts.URL+"/blah-blah-blqh", nil)
	if err != nil {
		panic(err)
	}

	rq3 := requester.New(http.Client{Timeout: 3 * time.Second}, lg.Middleware, clearHeaders)
	req.Header.Set("foo", "bar") // set header, will be removed by clearHeaders
	resp, err = rq3.Do(req)
	if err != nil {
		panic(err)
	}
	log.Printf("status: %s", resp.Status)
}

// custom middleware, clears "foo" header
func clearHeaders(next http.RoundTripper) http.RoundTripper {
	fn := func(r *http.Request) (*http.Response, error) {
		r.Header.Del("foo")
		return next.RoundTrip(r)
	}
	return middleware.RoundTripperFunc(fn)
}

func startTestServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("request: %+v", r)
		w.Header().Set("k1", "v1")
		w.Write([]byte("something"))
	}))
}
