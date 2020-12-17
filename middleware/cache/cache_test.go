package cache

import (
	"bytes"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/go-pkgz/requester/middleware/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_extractCacheKey(t *testing.T) {

	makeReq := func(method, url string, body io.Reader, headers http.Header) *http.Request {
		res, err := http.NewRequest(method, url, body)
		require.NoError(t, err)
		if headers != nil {
			res.Header = headers
		}
		return res
	}

	tbl := []struct {
		req     *http.Request
		opts    []func(m *Middleware)
		keyDbg  string
		keyHash string
	}{
		{
			req:     makeReq("GET", "http://example.com/1/2?k1=v1&k2=v2", nil, nil),
			opts:    []func(m *Middleware){},
			keyDbg:  "http://example.com/1/2?k1=v1&k2=v2##GET####",
			keyHash: "e847b72f947c83d096d71433f6d53202c148242d54150dc275e547f023ff3d5e",
		},
		{
			req: makeReq("GET", "http://example.com/1/2?k1=v1&k2=v2", nil,
				http.Header{"keyDbg": []string{"val1", "val2"}, "k2": []string{"v22"}}),
			opts:    []func(m *Middleware){KeyWithHeaders},
			keyDbg:  "http://example.com/1/2?k1=v1&k2=v2##GET##k2:v22$$keyDbg:val1%%val2##",
			keyHash: "7770dca95a1fe3a1dd5719dcc376e2dfa9f64a6c77729c8c98120db5d3ddf6ce",
		},
		{
			req: makeReq("GET", "http://example.com/1/2?k1=v1&k2=v2", nil,
				http.Header{"keyDbg": []string{"val1", "val2"}, "k2": []string{"v22"}}),
			opts:    []func(m *Middleware){KeyWithHeadersIncluded("k2")},
			keyDbg:  "http://example.com/1/2?k1=v1&k2=v2##GET##k2:v22##",
			keyHash: "96cdeaac00f84d5e80b9f8e57dceab324ee8d27e44f379c5150f315ba5a61dfb",
		},
		{
			req: makeReq("GET", "http://example.com/1/2?k1=v1&k2=v2", nil,
				http.Header{"keyDbg": []string{"val1", "val2"}, "k2": []string{"v22"}}),
			opts:    []func(m *Middleware){KeyWithHeadersExcluded("k2")},
			keyDbg:  "http://example.com/1/2?k1=v1&k2=v2##GET##keyDbg:val1%%val2##",
			keyHash: "7df35feb246b3cc39d15f2b86825dab6587044e017db5284613ce55b3d30dad5",
		},
		{
			req: makeReq("GET", "http://example.com/1/2?k1=v1&k2=v2", nil,
				http.Header{"keyDbg": []string{"val1", "val2"}, "k2": []string{"v22"}}),
			opts:    []func(m *Middleware){KeyWithHeadersExcluded("xyz", "abc")},
			keyDbg:  "http://example.com/1/2?k1=v1&k2=v2##GET##k2:v22$$keyDbg:val1%%val2##",
			keyHash: "7770dca95a1fe3a1dd5719dcc376e2dfa9f64a6c77729c8c98120db5d3ddf6ce",
		},
		{
			req: makeReq("GET", "http://example.com/1/2?k1=v1&k2=v2", bytes.NewBufferString("something"),
				http.Header{"keyDbg": []string{"val1", "val2"}, "k2": []string{"v22"}}),
			opts:    []func(m *Middleware){KeyWithHeadersExcluded("xyz", "abc")},
			keyDbg:  "http://example.com/1/2?k1=v1&k2=v2##GET##k2:v22$$keyDbg:val1%%val2##",
			keyHash: "7770dca95a1fe3a1dd5719dcc376e2dfa9f64a6c77729c8c98120db5d3ddf6ce",
		},
		{
			req: makeReq("GET", "http://example.com/1/2?k1=v1&k2=v2", bytes.NewBufferString("something"),
				http.Header{"keyDbg": []string{"val1", "val2"}, "k2": []string{"v22"}}),
			opts:    []func(m *Middleware){KeyWithHeadersExcluded("xyz", "abc"), KeyWithBody},
			keyDbg:  "http://example.com/1/2?k1=v1&k2=v2##GET##k2:v22$$keyDbg:val1%%val2##something",
			keyHash: "c77208b375a9df49e97920b5621c9ac8e733a13ab6c74abcef7bc4f052af8d38",
		},
		{
			req: makeReq("GET", "http://example.com/1/2?k1=v1&k2=v2", nil, nil),
			opts: []func(m *Middleware){KeyFunc(func(r *http.Request) string {
				return r.Host
			})},
			keyDbg:  "example.com",
			keyHash: "a379a6f6eeafb9a55e378c118034e2751e682fab9f2d30ab13d2125586ce1947",
		},
		{
			req: makeReq("GET", "http://example.com/1/2?k1=v1&k2=v2", nil, nil),
			opts: []func(m *Middleware){KeyFunc(func(r *http.Request) string {
				return r.URL.Path
			})},
			keyDbg:  "/1/2",
			keyHash: "c385023fa5c9b3d71679c9557649b476784a44c2f1f71b6d46a5a65694f688a0",
		},
	}

	//nolint scopelint
	for i, tt := range tbl {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			c := New(nil, tt.opts...)
			c.dbg = true
			keyDbg, err := c.extractCacheKey(tt.req)
			require.NoError(t, err)
			assert.Equal(t, tt.keyDbg, keyDbg)

			c.dbg = false
			keyHash, err := c.extractCacheKey(tt.req)
			require.NoError(t, err)
			assert.Equal(t, tt.keyHash, keyHash)

		})
	}

}

func TestMiddleware_Handle(t *testing.T) {

	cacheMock := mocks.CacheSvc{GetFunc: func(key string, fn func() (interface{}, error)) (interface{}, error) {
		return fn()
	}}
	c := New(&cacheMock)
	c.dbg = true

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("k1", "v1")
		_, err := w.Write([]byte("something"))
		require.NoError(t, err)
	}))

	client := http.Client{Transport: c.Middleware(http.DefaultTransport)}
	req, err := http.NewRequest("GET", ts.URL+"?k=v", nil)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	v, err := ioutil.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "something", string(v))
	assert.Equal(t, "v1", resp.Header.Get("k1"))
	assert.Equal(t, 1, len(cacheMock.GetCalls()))
	assert.Contains(t, cacheMock.GetCalls()[0].Key, "?k=v##GET####")

	req, err = http.NewRequest("GET", ts.URL+"?k=v", nil)
	require.NoError(t, err)

	resp, err = client.Do(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	v, err = ioutil.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "something", string(v))
	assert.Equal(t, 2, len(cacheMock.GetCalls()))
	assert.Contains(t, cacheMock.GetCalls()[1].Key, "?k=v##GET####")
}

func TestMiddleware_HandleMethodDisabled(t *testing.T) {
	cacheMock := mocks.CacheSvc{GetFunc: func(key string, fn func() (interface{}, error)) (interface{}, error) {
		return fn()
	}}
	c := New(&cacheMock, Methods("PUT"))
	c.dbg = true

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("k1", "v1")
		_, err := w.Write([]byte("something"))
		require.NoError(t, err)
	}))

	client := http.Client{Transport: c.Middleware(http.DefaultTransport)}
	req, err := http.NewRequest("GET", ts.URL+"?k=v", nil)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, 0, len(cacheMock.GetCalls()))

	req, err = http.NewRequest("PUT", ts.URL+"?k=v", nil)
	require.NoError(t, err)
	resp, err = client.Do(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, 1, len(cacheMock.GetCalls()))
}
