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

func TestShelly3EM(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		doc := MeterData{
			TotalPower: 1234,
		}
		v := doc.TotalPower / 3
		for i := 0; i < 3; i++ {
			doc.EMeters = append(doc.EMeters, struct {
				Current       float64 `json:"current"`
				IsValid       bool    `json:"is_valid"`
				PowerFactor   float64 `json:"pf"`
				Power         float64 `json:"power"`
				Total         float64 `json:"total"`
				TotalReturned float64 `json:"total_returned"`
				Voltage       float64 `json:"voltage"`
			}{
				Current:       v / 230,
				IsValid:       true,
				PowerFactor:   1.0,
				Power:         v,
				Total:         10,
				TotalReturned: 10,
				Voltage:       230,
			})
		}
		require.NoError(t, json.NewEncoder(w).Encode(doc))
	}))
	defer server.Close()

	url, _ := url.Parse(server.URL)
	shelly := Meter{Addr: url.Host, Client: http.DefaultClient}
	d, err := shelly.Read()
	require.NoError(t, err)

	assert.Equal(t, 1234.0, d.TotalPower)
	for _, e := range d.EMeters {
		assert.Equal(t, 1234.0/3, e.Power)
	}
}
