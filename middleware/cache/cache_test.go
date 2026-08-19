package cache

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/go-pkgz/requester/middleware/mocks"
)

func Test_extractCacheKey(t *testing.T) {
	makeReq := func(method, url string, body io.Reader, headers http.Header) *http.Request {
		res, err := http.NewRequest(method, url, body)
		assert.NoError(t, err)
		if headers != nil {
			res.Header = headers
		}
		return res
	}

	withHost := func(r *http.Request, host string) *http.Request {
		r.Host = host
		return r
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
			keyDbg:  "34:http://example.com/1/2?k1=v1&k2=v2##11:example.com##3:GET##0:##0:",
			keyHash: "b8850f76501e16a373f5b09ac8114d3aa4e7eb8a8ee661a69afaf31fb8fc5ae9",
		},
		{
			req: makeReq("GET", "http://example.com/1/2?k1=v1&k2=v2", nil,
				http.Header{"keyDbg": []string{"val1", "val2"}, "k2": []string{"v22"}}),
			opts:    []func(m *Middleware){KeyWithHeaders},
			keyDbg:  "34:http://example.com/1/2?k1=v1&k2=v2##11:example.com##3:GET##34:20:6:keyDbg4:val14:val29:2:k23:v22##0:",
			keyHash: "43cd56baf61e3473a41f14a8da6ec38bd756c4113b9488e7ce30ec2d6e1c7d9f",
		},
		{
			req: makeReq("GET", "http://example.com/1/2?k1=v1&k2=v2", nil,
				http.Header{"keyDbg": []string{"val1", "val2"}, "k2": []string{"v22"}}),
			opts:    []func(m *Middleware){KeyWithHeadersIncluded("k2")},
			keyDbg:  "34:http://example.com/1/2?k1=v1&k2=v2##11:example.com##3:GET##11:9:2:k23:v22##0:",
			keyHash: "e2232d5e2d796f7a4e08b6147f81b1052c49cbf6ef2aacb1f4c2373393ad6a86",
		},
		{
			req: makeReq("GET", "http://example.com/1/2?k1=v1&k2=v2", nil,
				http.Header{"keyDbg": []string{"val1", "val2"}, "k2": []string{"v22"}}),
			opts:    []func(m *Middleware){KeyWithHeadersExcluded("k2")},
			keyDbg:  "34:http://example.com/1/2?k1=v1&k2=v2##11:example.com##3:GET##23:20:6:keyDbg4:val14:val2##0:",
			keyHash: "6405827711ad3b35a9be9127b96b46a424180f64c1f1268d86929ad5939c3533",
		},
		{
			req: makeReq("GET", "http://example.com/1/2?k1=v1&k2=v2", nil,
				http.Header{"keyDbg": []string{"val1", "val2"}, "k2": []string{"v22"}}),
			opts:    []func(m *Middleware){KeyWithHeadersExcluded("xyz", "abc")},
			keyDbg:  "34:http://example.com/1/2?k1=v1&k2=v2##11:example.com##3:GET##34:20:6:keyDbg4:val14:val29:2:k23:v22##0:",
			keyHash: "43cd56baf61e3473a41f14a8da6ec38bd756c4113b9488e7ce30ec2d6e1c7d9f",
		},
		{
			req: makeReq("GET", "http://example.com/1/2?k1=v1&k2=v2", bytes.NewBufferString("something"),
				http.Header{"keyDbg": []string{"val1", "val2"}, "k2": []string{"v22"}}),
			opts:    []func(m *Middleware){KeyWithHeadersExcluded("xyz", "abc")},
			keyDbg:  "34:http://example.com/1/2?k1=v1&k2=v2##11:example.com##3:GET##34:20:6:keyDbg4:val14:val29:2:k23:v22##0:",
			keyHash: "43cd56baf61e3473a41f14a8da6ec38bd756c4113b9488e7ce30ec2d6e1c7d9f",
		},
		{
			req: makeReq("GET", "http://example.com/1/2?k1=v1&k2=v2", bytes.NewBufferString("something"),
				http.Header{"keyDbg": []string{"val1", "val2"}, "k2": []string{"v22"}}),
			opts:    []func(m *Middleware){KeyWithHeadersExcluded("xyz", "abc"), KeyWithBody},
			keyDbg:  "34:http://example.com/1/2?k1=v1&k2=v2##11:example.com##3:GET##34:20:6:keyDbg4:val14:val29:2:k23:v22##9:something",
			keyHash: "680b407586552fb94994bb00ed5915181f7473f39243aab8828f85deacb7ec15",
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
		{ // empty Host falls back to the URL host
			req:     withHost(makeReq("GET", "http://example.com/1/2?k1=v1&k2=v2", nil, nil), ""),
			opts:    []func(m *Middleware){},
			keyDbg:  "34:http://example.com/1/2?k1=v1&k2=v2##11:example.com##3:GET##0:##0:",
			keyHash: "b8850f76501e16a373f5b09ac8114d3aa4e7eb8a8ee661a69afaf31fb8fc5ae9",
		},
		{ // a crafted header value must not produce the key of the two separate headers below
			req: makeReq("GET", "http://example.com/1/2?k1=v1&k2=v2", nil,
				http.Header{"k1": []string{"v1$$k2:v2"}}),
			opts:    []func(m *Middleware){KeyWithHeaders},
			keyDbg:  "34:http://example.com/1/2?k1=v1&k2=v2##11:example.com##3:GET##18:15:2:k19:v1$$k2:v2##0:",
			keyHash: "060b3132211c60c51582a695c02248f35cdaec780c9c1c2099c752b620285410",
		},
		{
			req: makeReq("GET", "http://example.com/1/2?k1=v1&k2=v2", nil,
				http.Header{"k1": []string{"v1"}, "k2": []string{"v2"}}),
			opts:    []func(m *Middleware){KeyWithHeaders},
			keyDbg:  "34:http://example.com/1/2?k1=v1&k2=v2##11:example.com##3:GET##20:8:2:k12:v18:2:k22:v2##0:",
			keyHash: "997315632a3bdb27d0c0fb3b4741811c372c71e6a043d095cf3faaac6e1f9042",
		},
		{ // a multi-valued header must not produce the key of the two separate headers above
			req: makeReq("GET", "http://example.com/1/2?k1=v1&k2=v2", nil,
				http.Header{"k1": []string{"v1", "k2", "v2"}}),
			opts:    []func(m *Middleware){KeyWithHeaders},
			keyDbg:  "34:http://example.com/1/2?k1=v1&k2=v2##11:example.com##3:GET##19:16:2:k12:v12:k22:v2##0:",
			keyHash: "fa0c796430e540656c074c9163e0d66738a16c116fd4542d41eab2d19a185851",
		},
		{ // Host override makes the key differ from the same URL requested without it
			req:     withHost(makeReq("GET", "http://example.com/1/2?k1=v1&k2=v2", nil, nil), "other.example.com"),
			opts:    []func(m *Middleware){},
			keyDbg:  "34:http://example.com/1/2?k1=v1&k2=v2##17:other.example.com##3:GET##0:##0:",
			keyHash: "e75b27274cf89580f7428e711360f6a0e0bd305f22f80f0c88be1e794f32df67",
		},
		{ // Host repeating the URL host keeps the key of the plain request
			req:     withHost(makeReq("GET", "http://example.com/1/2?k1=v1&k2=v2", nil, nil), "example.com"),
			opts:    []func(m *Middleware){},
			keyDbg:  "34:http://example.com/1/2?k1=v1&k2=v2##11:example.com##3:GET##0:##0:",
			keyHash: "b8850f76501e16a373f5b09ac8114d3aa4e7eb8a8ee661a69afaf31fb8fc5ae9",
		},
	}

	// nolint scopelint
	for i, tt := range tbl {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			c := New(nil, tt.opts...)
			c.dbg = true
			keyDbg, err := c.extractCacheKey(tt.req)
			assert.NoError(t, err)
			assert.Equal(t, tt.keyDbg, keyDbg)

			c.dbg = false
			keyHash, err := c.extractCacheKey(tt.req)
			assert.NoError(t, err)
			assert.Equal(t, tt.keyHash, keyHash)

		})
	}

}

func TestMiddleware_Handle(t *testing.T) {
	cacheMock := mocks.CacheSvc{GetFunc: func(_ string, fn func() (interface{}, error)) (interface{}, error) {
		return fn()
	}}
	c := New(&cacheMock)
	c.dbg = true

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("k1", "v1")
		_, err := w.Write([]byte("something"))
		assert.NoError(t, err)
	}))

	client := http.Client{Transport: c.Middleware(http.DefaultTransport)}
	req, err := http.NewRequest("GET", ts.URL+"?k=v", http.NoBody)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	v, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "something", string(v))
	assert.Equal(t, "v1", resp.Header.Get("k1"))
	assert.Len(t, cacheMock.GetCalls(), 1)
	assert.Contains(t, cacheMock.GetCalls()[0].Key, "##"+strconv.Itoa(len(req.URL.Host))+":"+req.URL.Host+"##3:GET##")

	req, err = http.NewRequest("GET", ts.URL+"?k=v", http.NoBody)
	require.NoError(t, err)

	resp, err = client.Do(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	v, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "something", string(v))
	assert.Len(t, cacheMock.GetCalls(), 2)
	assert.Contains(t, cacheMock.GetCalls()[1].Key, "##"+strconv.Itoa(len(req.URL.Host))+":"+req.URL.Host+"##3:GET##")
}

func TestMiddleware_HandleMethodDisabled(t *testing.T) {
	cacheMock := mocks.CacheSvc{GetFunc: func(_ string, fn func() (interface{}, error)) (interface{}, error) {
		return fn()
	}}
	c := New(&cacheMock, Methods("PUT"))
	c.dbg = true

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("k1", "v1")
		_, err := w.Write([]byte("something"))
		assert.NoError(t, err)
	}))

	client := http.Client{Transport: c.Middleware(http.DefaultTransport)}
	req, err := http.NewRequest("GET", ts.URL+"?k=v", http.NoBody)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Empty(t, cacheMock.GetCalls())

	req, err = http.NewRequest("PUT", ts.URL+"?k=v", http.NoBody)
	require.NoError(t, err)
	resp, err = client.Do(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Len(t, cacheMock.GetCalls(), 1)
}

func TestMiddleware_EdgeCases(t *testing.T) {

	t.Run("nil service", func(t *testing.T) {
		c := New(nil)
		req, err := http.NewRequest("GET", "http://example.com", http.NoBody)
		require.NoError(t, err)
		resp, err := c.Middleware(http.DefaultTransport).RoundTrip(req)
		require.NoError(t, err)
		assert.NotNil(t, resp)
	})

	t.Run("large body", func(t *testing.T) {
		c := New(nil, KeyWithBody)
		originalBody := strings.Repeat("a", maxBodySize-1)
		body := bytes.NewBufferString(originalBody)
		req, err := http.NewRequest("POST", "http://example.com", body)
		require.NoError(t, err)
		key, err := c.extractCacheKey(req)
		require.NoError(t, err)
		assert.NotEmpty(t, key)

		// verify key was generated with truncated body but original body is still readable
		data, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		assert.Equal(t, originalBody, string(data))

		// get key again with same input
		body = bytes.NewBufferString(originalBody)
		req, err = http.NewRequest("POST", "http://example.com", body)
		require.NoError(t, err)
		key2, err := c.extractCacheKey(req)
		require.NoError(t, err)

		// verify keys match even with truncated bodies
		assert.Equal(t, key, key2, "keys should match for same content even if truncated")
	})

	t.Run("body read error", func(t *testing.T) {
		c := New(nil, KeyWithBody)
		errReader := &errorReader{err: errors.New("read error")}
		req, err := http.NewRequest("POST", "http://example.com", errReader)
		require.NoError(t, err)
		_, err = c.extractCacheKey(req)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "read error")
	})

	t.Run("body and headers", func(t *testing.T) {
		c := New(nil, KeyWithBody, KeyWithHeaders)
		body := bytes.NewBufferString("test body")
		req, err := http.NewRequest("POST", "http://example.com", body)
		require.NoError(t, err)
		req.Header.Set("Test", "value")
		key1, err := c.extractCacheKey(req)
		require.NoError(t, err)

		// same request but different header
		body = bytes.NewBufferString("test body")
		req, err = http.NewRequest("POST", "http://example.com", body)
		require.NoError(t, err)
		req.Header.Set("Test", "different")
		key2, err := c.extractCacheKey(req)
		require.NoError(t, err)

		assert.NotEqual(t, key1, key2)
	})
}

type errorReader struct {
	err error
}

func (e *errorReader) Read(_ []byte) (n int, err error) {
	return 0, e.err
}
