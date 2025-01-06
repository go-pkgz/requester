package logger

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/go-pkgz/requester/middleware/mocks"
)

func TestMiddleware_Handle(t *testing.T) {
	outBuf := bytes.NewBuffer(nil)
	loggerMock := &mocks.LoggerSvc{
		LogfFunc: func(format string, args ...interface{}) {
			_, _ = fmt.Fprintf(outBuf, format, args...)
		},
	}
	l := New(loggerMock)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("k1", "v1")
		_, err := w.Write([]byte("something"))
		require.NoError(t, err)
	}))

	client := http.Client{Transport: l.Middleware(http.DefaultTransport)}
	req, err := http.NewRequest("GET", ts.URL+"?k=v", http.NoBody)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	t.Log(outBuf.String())
	assert.True(t, strings.HasPrefix(outBuf.String(), "GET http://127.0.0.1:"))
	assert.Contains(t, outBuf.String(), "time:")

	assert.Equal(t, 1, len(loggerMock.LogfCalls()))
}

func TestMiddleware_HandleWithOptions(t *testing.T) {
	outBuf := bytes.NewBuffer(nil)
	loggerMock := &mocks.LoggerSvc{
		LogfFunc: func(format string, args ...interface{}) {
			_, _ = fmt.Fprintf(outBuf, format, args...)
		},
	}
	l := New(loggerMock, WithBody, WithHeaders, Prefix("HIT"))

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("k1", "v1")
		_, err := w.Write([]byte("something"))
		require.NoError(t, err)
	}))

	client := http.Client{Transport: l.Middleware(http.DefaultTransport)}
	req, err := http.NewRequest("POST", ts.URL+"?k=v", bytes.NewBufferString("blah1 blah2\nblah3"))
	require.NoError(t, err)
	req.Header.Set("inkey", "inval")

	resp, err := client.Do(req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	t.Log(outBuf.String())
	assert.True(t, strings.HasPrefix(outBuf.String(), "HIT POST http://127.0.0.1:"))
	assert.Contains(t, outBuf.String(), "time:")
	assert.Contains(t, outBuf.String(), `{"Inkey":["inval"]}`)
	assert.Contains(t, outBuf.String(), `body: blah1 blah2 blah3`)

	assert.Equal(t, 1, len(loggerMock.LogfCalls()))
}
func TestLogger_EdgeCases(t *testing.T) {
	t.Run("non-standard headers", func(t *testing.T) {
		outBuf := bytes.NewBuffer(nil)
		loggerMock := &mocks.LoggerSvc{
			LogfFunc: func(format string, args ...interface{}) {
				_, _ = fmt.Fprintf(outBuf, format, args...)
			},
		}
		l := New(loggerMock, WithHeaders)

		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("k1", "v1")
			_, err := w.Write([]byte("ok"))
			require.NoError(t, err)
		}))
		defer ts.Close()

		req, err := http.NewRequest("GET", ts.URL+"?k=v", http.NoBody)
		require.NoError(t, err)

		// use complex unicode char sequence that might affect json marshaling
		req.Header.Set("Test-Header", "привет世界")
		req.Header.Set("Multiple-Values", "val1")
		req.Header.Add("Multiple-Values", "val2")

		client := http.Client{Transport: l.Middleware(http.DefaultTransport)}
		resp, err := client.Do(req)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)

		logOutput := outBuf.String()
		assert.Contains(t, logOutput, "Test-Header")
		assert.Contains(t, logOutput, "привет世界")
		assert.Contains(t, logOutput, "Multiple-Values")
		assert.Contains(t, logOutput, "val1")
		assert.Contains(t, logOutput, "val2")
	})

	t.Run("prefix handling", func(t *testing.T) {
		outBuf := bytes.NewBuffer(nil)
		loggerMock := &mocks.LoggerSvc{
			LogfFunc: func(format string, args ...interface{}) {
				_, _ = fmt.Fprintf(outBuf, format, args...)
			},
		}
		l := New(loggerMock, Prefix("TEST-PREFIX"))

		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, err := w.Write([]byte("ok"))
			require.NoError(t, err)
		}))
		defer ts.Close()

		client := http.Client{Transport: l.Middleware(http.DefaultTransport)}
		resp, err := client.Get(ts.URL)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)

		assert.True(t, strings.HasPrefix(outBuf.String(), "TEST-PREFIX"))
	})

	t.Run("large body truncation", func(t *testing.T) {
		outBuf := bytes.NewBuffer(nil)
		loggerMock := &mocks.LoggerSvc{
			LogfFunc: func(format string, args ...interface{}) {
				_, _ = fmt.Fprintf(outBuf, format, args...)
			},
		}
		l := New(loggerMock, WithBody)

		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, err := w.Write([]byte("ok"))
			require.NoError(t, err)
		}))
		defer ts.Close()

		largeBody := strings.Repeat("x", 2000)
		req, err := http.NewRequest("POST", ts.URL, strings.NewReader(largeBody))
		require.NoError(t, err)

		client := http.Client{Transport: l.Middleware(http.DefaultTransport)}
		resp, err := client.Do(req)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)

		output := outBuf.String()
		assert.Contains(t, output, "...")
		assert.True(t, len(output) < len(largeBody))
	})

	t.Run("multiline body", func(t *testing.T) {
		outBuf := bytes.NewBuffer(nil)
		loggerMock := &mocks.LoggerSvc{
			LogfFunc: func(format string, args ...interface{}) {
				_, _ = fmt.Fprintf(outBuf, format, args...)
			},
		}
		l := New(loggerMock, WithBody)

		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, err := w.Write([]byte("ok"))
			require.NoError(t, err)
		}))
		defer ts.Close()

		bodyContent := "line1\nline2\nline3"
		req, err := http.NewRequest("POST", ts.URL, strings.NewReader(bodyContent))
		require.NoError(t, err)

		client := http.Client{Transport: l.Middleware(http.DefaultTransport)}
		resp, err := client.Do(req)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)

		output := outBuf.String()
		assert.NotContains(t, output, "\n")
		assert.Contains(t, output, "line1 line2 line3")
	})

	t.Run("nil logger", func(t *testing.T) {
		l := New(nil, WithBody)
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, err := w.Write([]byte("ok"))
			require.NoError(t, err)
		}))
		defer ts.Close()

		req, err := http.NewRequest("POST", ts.URL, strings.NewReader("test"))
		require.NoError(t, err)

		client := http.Client{Transport: l.Middleware(http.DefaultTransport)}
		resp, err := client.Do(req)
		require.NoError(t, err)
		assert.Equal(t, 200, resp.StatusCode)
	})
}
