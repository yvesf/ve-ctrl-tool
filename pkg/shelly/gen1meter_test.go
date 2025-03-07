package shelly

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGen1Meter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		doc := Gen1MeterData{
			TotalPower_: 1234,
		}
		require.NoError(t, json.NewEncoder(w).Encode(doc))
	}))
	defer server.Close()

	url, _ := url.Parse(server.URL)
	shelly := Gen1Meter{Addr: url.Host, Client: http.DefaultClient}
	d, err := shelly.Read()
	require.NoError(t, err)

	assert.Equal(t, 1234.0, d.TotalPower())
}
