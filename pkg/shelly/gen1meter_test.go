package shelly

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/carlmjohnson/be"
)

func TestGen1Meter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		doc := Gen1MeterData{
			TotalPowerFloat: 1234,
		}
		be.NilErr(t, json.NewEncoder(w).Encode(doc))
	}))
	defer server.Close()

	url, _ := url.Parse(server.URL)
	shelly := Gen1Meter{Addr: url.Host, Client: http.DefaultClient}
	d, err := shelly.Read()
	be.NilErr(t, err)
	be.Equal(t, 1234.0, d.TotalPower())
}
