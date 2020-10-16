# Requester

[![Build Status](https://github.com/go-pkgz/repeater/workflows/build/badge.svg)](https://github.com/go-pkgz/requester/actions) [![Go Report Card](https://goreportcard.com/badge/github.com/go-pkgz/requester)](https://goreportcard.com/report/github.com/go-pkgz/requester) [![Coverage Status](https://coveralls.io/repos/github/go-pkgz/requester/badge.svg?branch=master)](https://coveralls.io/github/go-pkgz/requester?branch=master)


Package provides very thin wrapper (no external dependencies) for `http.Client` allowing to use layers (middlewares) on `http.RoundTripper` level. 
The goal is to keep the way users leverage stdlib http client but add a few useful extras. 

_Pls note: this is not a replacement for http.Client but rather a companion library._

```go
    rq := requester.New(                        // make requester
        http.Client{Timeout: 5*time.Second},    // set http client
        requester.MaxConcurrent(8),             // maximum number of concurrent requests
        requester.JSON,                         // set json headers
        requester.Header("X-AUTH", "123456789"),// set auth header
        requester.Logger(requester.StdLogger),  // enable logging
    )
    
    req := http.NewRequest("GET", "http://example.com/api", nil) // create the usual http.Request
    req.Header.Set("foo", "bar") // prepare headers, as usual
    resp, err := rq.Do(req) // instead of client.Do call requester.Do
```


## Install and update

`go get -u github.com/go-pkgz/requester`


## `Requester` middlewares:

- `Header` - appends user-defined headers to all requests. 
- `JSOM` - sets headers `"Content-Type": "application/json"` and `"Accept": "application/json"`
- `BasicAuth(user, passwd string)` - adds HTTP Basic Authentication
- `MaxConcurrent` - sets maximum concurrency
- `Repeater` - sets repeater to retry failed requests. Doesn't provide repeater implementation but wraps it. Compatible with any repeater (for example [go-pkgz/repeater](https://github.com/go-pkgz/repeater)) implementing a single method interface `Do(ctx context.Context, fun func() error, errors ...error) (err error)` interface. 
- `Cache` - sets any `LoadingCache` implementation to be used for request/response caching. Doesn't provide cache, but wraps it. Compatible with any cache (for example a family of caches from [go-pkgz/lcw](https://github.com/go-pkgz/lcw)) implementing a single-method interface `Get(key string, fn func() (interface{}, error)) (val interface{}, err error)`
- `Logger` - sets logger, compatible with any implementation  of a single-method interface `Logf(format string, args ...interface{})`, for example [go-pkgz/lgr](https://github.com/go-pkgz/lgr)
- `CircuitBreaker` - sets circuit breaker, interface compatible with [sony/gobreaker](https://github.com/sony/gobreaker)

User can add any custom middleware. All it needs is a handler `RoundTripperHandler func(http.RoundTripper) http.RoundTripper`. 
Convenient functional adapter `middleware.RoundTripperFunc` provided.
 
### Logging 

Logger should implement `Logger` interface with a single method `Logf(format string, args ...interface{})`. 
For convince func type `LoggerFunc` provided as an adapter to allow the use of ordinary functions as `Logger`. 

Two basic implementation included: 

- `NoOpLogger` do-nothing logger (default) 
- `StdLogger` wrapper for stdlib logger.

logging options:

- `Prefix(prefix string)` sets prefix for each logged line
- `WithBody` - allows request's body logging
- `WithHeaders` - allows request's headers logging

Note: if logging allowed it will log url, method and may log headers and the request body. 
This may affect application security, for example if request pass some sensitive info as a part of body or header. 
In this case consider turning logging off or provide your own logger suppressing all you need to hide. 

### MaxConcurrent

MaxConcurrent middleware can be used to limit concurrency of a given requester as well as to limit overall concurrency for multiple
requesters. For the first case `MaxConcurrent(N)` should be created in the requester chain of middlewares, for example `rq := requester.New(http.Client{Timeout: 3 * time.Second}, middleware.MaxConcurrent(8))`. To make it global `MaxConcurrent` should be created once, outside of the chain and passed into each requester, i.e.

```go
mc := middleware.MaxConcurrent(16)
rq1 := requester.New(http.Client{Timeout: 3 * time.Second}, mc)
rq2 := requester.New(http.Client{Timeout: 1 * time.Second}, middleware.JSON, mc)
```

### Cache

Cache expects `LoadingCache` interface implementing a single method:
`Get(key string, fn func() (interface{}, error)) (val interface{}, err error)`. [LCW](https://github.com/go-pkgz/lcw/) can 
be used directly, and in order to adopt other caches see provided `LoadingCacheFunc`.

#### caching key and allowed requests

By default only `GET` calls will be cached. This can be changed with `Methods(methods ...string)` option.
The default key composed from the full URL.

There are several options defining what part of the request will be used for the key:

- `KeyWithHeaders` - adds all headers to a key
- `KeyWithHeadersIncluded(headers ...string)` - adds only requested headers
- `KeyWithHeadersExcluded(headers ...string) ` - adds all headers excluded
- `KeyWithBody` - adds request's body, limited to the first 16k of the body
- `KeyFunc` - any custom logic provided by caller

example: `cache.New(lruCache, cache.Methods("GET", "POST"), cache.KeyFunc() {func(r *http.Request) string {return r.Host})`


#### cache and streaming response

`Cache` is **not compatible** with http streaming mode. Practically this is rare and exotic case, but 
allowing `Cache` will effectively transform streaming response to "get all" typical response. This is because cache
has to read response body fully in order to save it, so technically streaming will be working but client will get
all the data at once. 

### Repeater

`Repeater` expects a single method interface `Do(fn func() error, stopOnErrs ...error) (err error)`. [repeater](github.com/go-pkgz/repeater) can be used directly.

By default repeater will retry on any error and any status code. However, user can pass `stopOnErrs` codes to eliminate retries, 
for example: `Repeater(repeaterSvc, 500, 400)` won't repeat on 500 and 400 statuses.

### User-Defined Middlewares

User can add any additional handlers (middleware) to the chain. Each middleware provides `middleware.RoundTripperHandler` and
can alter the request or implement any other custom functionality.

Example of a handler resetting particular header:

```go
maskHeader := func(http.RoundTripper) http.RoundTripper {
    fn := func(req *http.Request) (*http.Response, error) {
        req.Header.Del("deleteme")
        return next(req)
    }
    return middleware.RoundTripperFunc(fn)
}

rq := requester.New(http.Client{}, maskHeader)
```

## Adding middleware to requester

There are 3 ways to add middleware(s):

- Pass it to `New` constructor, i.e. `requester.New(http.Client{}, middleware.MaxConcurrent(8), middleware.Header("foo", "bar"))`
- Add after construction with `Use` method
- Create new, inherited requester by using `With`:
```go
rq := requester.New(http.Client{}, middleware.Header("foo", "bar")) // make requester enforcing header foo:bar
resp, err := rq.Do(some_http_req) // send a request

rqLimited := rq.With(middleware.MaxConcurrent(8)) // make requester from rq (foo:bar enforced) and add 8 max concurrency
resp, err := rqLimited.Do(some_http_req)
```

## Getting http.Client with all middlewares

For convenience `requester.Client()` returns `*http.Client` with all middlewares injected in.   

## Helpers and adapters

- `CircuitBreakerFunc func(req func() (interface{}, error)) (interface{}, error)` - adapter to allow the use of an ordinary functions as CircuitBreakerSvc.
- `logger.Func func(format string, args ...interface{})` - functional adapter for `logger.Service`.
- `cache.ServiceFunc func(key string, fn func() (interface{}, error)) (interface{}, error)` - functional adapter for `cache.Service`.
- `RoundTripperFunc func(*http.Request) (*http.Response, error)` - functional adapter for RoundTripperHandler