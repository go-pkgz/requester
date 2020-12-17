package logger

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-pkgz/requester/middleware/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMiddleware_Handle(t *testing.T) {
	outBuf := bytes.NewBuffer(nil)
	loggerMock := &mocks.LoggerSvc{
		LogfFunc: func(format string, args ...interface{}) {
			_, _ = outBuf.WriteString(fmt.Sprintf(format, args...))
		},
	}
	l := New(loggerMock)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("k1", "v1")
		_, err := w.Write([]byte("something"))
		require.NoError(t, err)
	}))

	client := http.Client{Transport: l.Middleware(http.DefaultTransport)}
	req, err := http.NewRequest("GET", ts.URL+"?k=v", nil)
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
			_, _ = outBuf.WriteString(fmt.Sprintf(format, args...))
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
