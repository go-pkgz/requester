package middleware

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/go-pkgz/requester/middleware/mocks"
)

func TestRepeater_Passed(t *testing.T) {

	var count int32
	rmock := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
		if atomic.AddInt32(&count, 1) >= 3 {
			resp := &http.Response{StatusCode: 201}
			return resp, nil
		}
		resp := &http.Response{StatusCode: 400, Status: "400 Bad Request"}
		return resp, errors.New("blah")
	}}

	repeater := &mocks.RepeaterSvcMock{DoFunc: func(ctx context.Context, fun func() error, errs ...error) (err error) {
		for i := 0; i < 5; i++ {
			if err = fun(); err == nil {
				return nil
			}
		}
		return err
	}}

	h := Repeater(repeater)

	req, err := http.NewRequest("GET", "http://example.com/blah", http.NoBody)
	require.NoError(t, err)

	resp, err := h(rmock).RoundTrip(req)
	require.NoError(t, err)
	assert.Equal(t, 201, resp.StatusCode)

	assert.Equal(t, 3, rmock.Calls())
}

func TestRepeater_Failed(t *testing.T) {

	rmock := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
		resp := &http.Response{StatusCode: 400}
		return resp, errors.New("http error")
	}}

	repeater := &mocks.RepeaterSvcMock{DoFunc: func(ctx context.Context, fun func() error, errs ...error) (err error) {
		for i := 0; i < 5; i++ {
			if err = fun(); err == nil {
				return nil
			}
		}
		return err
	}}

	h := Repeater(repeater)

	req, err := http.NewRequest("GET", "http://example.com/blah", http.NoBody)
	require.NoError(t, err)

	_, err = h(rmock).RoundTrip(req)
	require.EqualError(t, err, "repeater: http error")

	assert.Equal(t, 5, rmock.Calls())
}

func TestRepeater_FailedStatus(t *testing.T) {

	rmock := &mocks.RoundTripper{RoundTripFunc: func(r *http.Request) (*http.Response, error) {
		resp := &http.Response{StatusCode: 400, Status: "400 Bad Request"}
		return resp, nil
	}}

	repeater := &mocks.RepeaterSvcMock{DoFunc: func(ctx context.Context, fun func() error, errs ...error) (err error) {
		for i := 0; i < 5; i++ {
			if err = fun(); err == nil {
				return nil
			}
		}
		return err
	}}

	{
		h := Repeater(repeater, 300, 400, 401)

		req, err := http.NewRequest("GET", "http://example.com/blah", http.NoBody)
		require.NoError(t, err)

		_, err = h(rmock).RoundTrip(req)
		require.EqualError(t, err, "repeater: 400 Bad Request")
	}

	assert.Equal(t, 5, rmock.Calls())

	{
		h := Repeater(repeater, 300, 401)

		req, err := http.NewRequest("GET", "http://example.com/blah", http.NoBody)
		require.NoError(t, err)

		resp, err := h(rmock).RoundTrip(req)
		require.NoError(t, err)
		assert.Equal(t, 400, resp.StatusCode)
	}

}
