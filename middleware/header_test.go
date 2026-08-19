package middleware

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/go-pkgz/requester/middleware/mocks"
)

func TestHeader(t *testing.T) {
	rmock := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
		assert.Equal(t, "v1", r.Header.Get("k1"))
		resp := &http.Response{StatusCode: 201}
		return resp, nil
	}}

	req, err := http.NewRequest("GET", "http://example.com/blah", http.NoBody)
	require.NoError(t, err)

	h := Header("k1", "v1")
	resp, err := h(rmock).RoundTrip(req)
	require.NoError(t, err)
	assert.Equal(t, 201, resp.StatusCode)

	assert.Equal(t, 1, rmock.Calls())
}

func TestJSON(t *testing.T) {
	rmock := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "application/json", r.Header.Get("Accept"))
		resp := &http.Response{StatusCode: 201}
		return resp, nil
	}}

	req, err := http.NewRequest("GET", "http://example.com/blah", http.NoBody)
	require.NoError(t, err)

	resp, err := JSON(rmock).RoundTrip(req)
	require.NoError(t, err)
	assert.Equal(t, 201, resp.StatusCode)
	assert.Equal(t, 1, rmock.Calls())
}

func TestBasicAuth(t *testing.T) {
	rmock := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
		assert.Equal(t, "Basic dXNlcjpwYXNzd2Q=", r.Header.Get("Authorization"))
		resp := &http.Response{StatusCode: 201}
		return resp, nil
	}}

	req, err := http.NewRequest("GET", "http://example.com/blah", http.NoBody)
	require.NoError(t, err)

	resp, err := BasicAuth("user", "passwd")(rmock).RoundTrip(req)
	require.NoError(t, err)
	assert.Equal(t, 201, resp.StatusCode)
	assert.Equal(t, 1, rmock.Calls())
}
func TestHeader_EdgeCases(t *testing.T) {
	t.Run("case insensitive", func(t *testing.T) {
		rmock := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
			assert.Equal(t, "v1", r.Header.Get("key"))
			assert.Equal(t, "v1", r.Header.Get("Key"))
			assert.Equal(t, "v1", r.Header.Get("KEY"))
			return &http.Response{StatusCode: 200}, nil
		}}

		req, err := http.NewRequest("GET", "http://example.com", http.NoBody)
		require.NoError(t, err)

		h := Header("KEY", "v1")
		_, err = h(rmock).RoundTrip(req)
		require.NoError(t, err)
	})

	t.Run("header overwrite", func(t *testing.T) {
		rmock := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
			// header middleware overwrites existing values
			assert.Equal(t, []string{"v2"}, r.Header.Values("key"))
			return &http.Response{StatusCode: 200}, nil
		}}

		req, err := http.NewRequest("GET", "http://example.com", http.NoBody)
		require.NoError(t, err)
		req.Header.Add("key", "v1")

		h := Header("key", "v2")
		_, err = h(rmock).RoundTrip(req)
		require.NoError(t, err)
	})

	t.Run("json headers set", func(t *testing.T) {
		rmock := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
			// JSON middleware sets both Content-Type and Accept
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
			assert.Equal(t, "application/json", r.Header.Get("Accept"))
			return &http.Response{StatusCode: 200}, nil
		}}

		req, err := http.NewRequest("GET", "http://example.com", http.NoBody)
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/xml")

		resp, err := JSON(rmock).RoundTrip(req)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
	})

	t.Run("basic auth headers", func(t *testing.T) {
		rmock := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
			user, pass, ok := r.BasicAuth()
			assert.True(t, ok)
			assert.Equal(t, "user", user)
			assert.Equal(t, "pass123$$!@", pass)
			return &http.Response{StatusCode: 200}, nil
		}}

		req, err := http.NewRequest("GET", "http://example.com", http.NoBody)
		require.NoError(t, err)

		h := BasicAuth("user", "pass123$$!@")
		resp, err := h(rmock).RoundTrip(req)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
	})

	t.Run("header middleware order", func(t *testing.T) {
		rmock := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
			assert.Equal(t, "v1", r.Header.Get("key1"))
			assert.Equal(t, "v2", r.Header.Get("key2"))
			// JSON middleware runs last in the chain, so it overwrites Content-Type
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
			user, pass, ok := r.BasicAuth()
			assert.True(t, ok)
			assert.Equal(t, "user", user)
			assert.Equal(t, "pass", pass)
			return &http.Response{StatusCode: 200}, nil
		}}

		req, err := http.NewRequest("GET", "http://example.com", http.NoBody)
		require.NoError(t, err)

		h1 := Header("key1", "v1")
		h2 := Header("key2", "v2")
		h3 := BasicAuth("user", "pass")
		h4 := JSON

		resp, err := h1(h2(h3(h4(rmock)))).RoundTrip(req)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
	})

	t.Run("empty headers", func(t *testing.T) {
		rmock := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
			assert.Empty(t, r.Header.Get("empty-key"))
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
			return &http.Response{StatusCode: 200}, nil
		}}

		req, err := http.NewRequest("GET", "http://example.com", http.NoBody)
		require.NoError(t, err)

		h := Header("empty-key", "")
		_, err = h(JSON(rmock)).RoundTrip(req)
		require.NoError(t, err)
	})
}

func TestHeader_Redirects(t *testing.T) {
	// chain makes a request as the standard client does it for a redirect, i.e. every hop
	// pointing to the response of the previous one
	chain := func(t *testing.T, urls ...string) *http.Request {
		t.Helper()
		var prev *http.Request
		for _, u := range urls {
			req, err := http.NewRequest("GET", u, http.NoBody)
			require.NoError(t, err)
			if prev != nil {
				req.Response = &http.Response{StatusCode: 302, Request: prev}
			}
			prev = req
		}
		return prev
	}

	tbl := []struct {
		name string
		urls []string
		kept bool
	}{
		{"no redirect", []string{"http://example.com/a"}, true},
		{"same host", []string{"http://example.com/a", "http://example.com/b"}, true},
		{"subdomain of the original host", []string{"http://example.com/a", "http://sub.example.com/b"}, true},
		{"another port on the original host", []string{"http://example.com/a", "http://example.com:8443/b"}, true},
		{"host in a different case", []string{"http://example.com/a", "http://EXAMPLE.com/b"}, true},
		{"scheme change on the original host", []string{"http://example.com/a", "https://example.com/b"}, true},
		{"another host", []string{"http://example.com/a", "http://attacker.com/b"}, false},
		{"parent of the original host", []string{"http://sub.example.com/a", "http://example.com/b"}, false},
		{"host with the original as a suffix", []string{"http://example.com/a", "http://notexample.com/b"}, false},
		{"back on the original host after leaving it",
			[]string{"http://example.com/a", "http://attacker.com/b", "http://example.com/c"}, false},
		{"two hops on the original host",
			[]string{"http://example.com/a", "http://sub.example.com/b", "http://example.com/c"}, true},
	}

	for _, tt := range tbl {
		t.Run(tt.name, func(t *testing.T) {
			var got http.Header
			rmock := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
				got = r.Header.Clone()
				return &http.Response{StatusCode: 200}, nil
			}}

			req := chain(t, tt.urls...)
			// the client copies non-sensitive headers to the next hop, the middleware's own value must not survive it
			req.Header.Set("Authorization", "Basic dXNlcjpwYXNzd2Q=")
			req.Header.Set("X-Auth", "secret")

			h := BasicAuth("user", "passwd")(SecretHeader("X-Auth", "secret")(Header("X-Trace", "t1")(rmock)))
			_, err := h.RoundTrip(req)
			require.NoError(t, err)

			assert.Equal(t, "t1", got.Get("X-Trace"), "plain header goes on every hop")
			if tt.kept {
				assert.Equal(t, "Basic dXNlcjpwYXNzd2Q=", got.Get("Authorization"))
				assert.Equal(t, "secret", got.Get("X-Auth"))
				return
			}
			assert.Empty(t, got.Get("Authorization"))
			assert.Empty(t, got.Get("X-Auth"))
		})
	}

	t.Run("values of the destination kept on another host", func(t *testing.T) {
		var got http.Header
		rmock := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
			got = r.Header.Clone()
			return &http.Response{StatusCode: 200}, nil
		}}

		req := chain(t, "http://example.com/a", "http://attacker.com/b")
		req.Header.Set("Authorization", "Basic dXNlcjpwYXNzd2Q=") // the middleware's own value
		req.Header.Add("Authorization", "Bearer for-the-destination")
		req.Header.Set("Cookie", "set=by-the-jar")

		h := BasicAuth("user", "passwd")(SecretHeader("Cookie", "set=by-the-middleware")(rmock))
		_, err := h.RoundTrip(req)
		require.NoError(t, err)

		assert.Equal(t, []string{"Bearer for-the-destination"}, got.Values("Authorization"))
		assert.Equal(t, []string{"set=by-the-jar"}, got.Values("Cookie"))
	})

	t.Run("caller value of a secret header dropped on another host", func(t *testing.T) {
		var got http.Header
		rmock := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
			got = r.Header.Clone()
			return &http.Response{StatusCode: 200}, nil
		}}

		// the client copies headers it doesn't treat as credentials from the original request to every hop
		req := chain(t, "http://example.com/a", "http://attacker.com/b")
		req.Header.Set("X-Auth", "set-by-the-caller")

		_, err := SecretHeader("X-Auth", "set-by-the-middleware")(rmock).RoundTrip(req)
		require.NoError(t, err)
		assert.Empty(t, got.Values("X-Auth"))
	})

	t.Run("redirect with unknown origin", func(t *testing.T) {
		var got http.Header
		rmock := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
			got = r.Header.Clone()
			return &http.Response{StatusCode: 200}, nil
		}}

		req, err := http.NewRequest("GET", "http://example.com/b", http.NoBody)
		require.NoError(t, err)
		req.Response = &http.Response{StatusCode: 302} // transport left Request unset, the origin can't be established

		_, err = BasicAuth("user", "passwd")(rmock).RoundTrip(req)
		require.NoError(t, err)
		assert.Empty(t, got.Get("Authorization"))
	})

	t.Run("credential header set through Header", func(t *testing.T) {
		var got http.Header
		rmock := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
			got = r.Header.Clone()
			return &http.Response{StatusCode: 200}, nil
		}}

		h := Header("authorization", "Bearer t")(Header("Cookie", "s=1")(rmock))
		_, err := h.RoundTrip(chain(t, "http://example.com/a", "http://attacker.com/b"))
		require.NoError(t, err)
		assert.Empty(t, got.Get("Authorization"))
		assert.Empty(t, got.Get("Cookie"))
	})
}

func TestHeader_RedirectsThroughClient(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := fmt.Fprintf(w, "%s|%s|%s", r.Header.Get("Authorization"), r.Header.Get("X-Auth"), r.Header.Get("X-Trace"))
		assert.NoError(t, err)
	}))
	defer target.Close()

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://"+strings.TrimPrefix(r.URL.Path, "/to/")+"/target", http.StatusFound)
	}))
	defer origin.Close()

	// the middleware decides on the hostname, so the test hostnames resolve to the local servers
	hosts := map[string]string{
		"origin.example:80":     origin.Listener.Addr().String(),
		"attacker.example:80":   target.Listener.Addr().String(),
		"sub.origin.example:80": target.Listener.Addr().String(),
	}
	base := &http.Transport{DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
		if mapped, ok := hosts[addr]; ok {
			addr = mapped
		}
		return (&net.Dialer{}).DialContext(ctx, network, addr)
	}}
	client := http.Client{Transport: BasicAuth("user", "passwd")(SecretHeader("X-Auth", "secret")(Header("X-Trace", "t1")(base)))}

	got := func(t *testing.T, redirectTo string) (auth, secret, trace string) {
		t.Helper()
		resp, err := client.Get("http://origin.example/to/" + redirectTo)
		require.NoError(t, err)
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		parts := strings.Split(string(body), "|")
		require.Len(t, parts, 3)
		return parts[0], parts[1], parts[2]
	}

	t.Run("credentials dropped on another host", func(t *testing.T) {
		auth, secret, trace := got(t, "attacker.example")
		assert.Empty(t, auth)
		assert.Empty(t, secret)
		assert.Equal(t, "t1", trace, "plain header still goes through")
	})

	t.Run("credentials kept on a subdomain of the original host", func(t *testing.T) {
		auth, secret, trace := got(t, "sub.origin.example")
		assert.Equal(t, "Basic dXNlcjpwYXNzd2Q=", auth)
		assert.Equal(t, "secret", secret)
		assert.Equal(t, "t1", trace)
	})
}
