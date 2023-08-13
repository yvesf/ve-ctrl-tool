package shelly_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yvesf/ve-ctrl-tool/pkg/shelly"
)

func TestShelly1(t *testing.T) {
	var req *http.Request
	var respBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req = r
		_, _ = w.Write(respBody)
	}))
	defer server.Close()

	serverURL, _ := url.Parse(server.URL)

	s := shelly.Relay{
		Addr:   serverURL.Host,
		Client: http.DefaultClient,
	}

	// Get
	_, err := s.Get()
	require.EqualError(t, err, "failed to decode response: EOF")

	respBody = []byte(`{"ison":false}`)
	state, err := s.Get()
	require.NoError(t, err)
	require.False(t, state)
	require.Equal(t, `/relay/0`, req.URL.Path)
	require.Empty(t, req.URL.RawQuery)

	respBody = []byte(`{"ison":true}`)
	state, err = s.Get()
	require.NoError(t, err)
	require.True(t, state)
	require.Equal(t, `/relay/0`, req.URL.Path)
	require.Empty(t, req.URL.RawQuery)

	// Set
	respBody = []byte(`{}`)
	err = s.Set(true)
	require.NoError(t, err)
	require.Equal(t, `/relay/0`, req.URL.Path)
	require.Equal(t, `turn=on`, req.URL.RawQuery)

	err = s.Set(false)
	require.NoError(t, err)
	require.Equal(t, `/relay/0`, req.URL.Path)
	require.Equal(t, `turn=off`, req.URL.RawQuery)
}
