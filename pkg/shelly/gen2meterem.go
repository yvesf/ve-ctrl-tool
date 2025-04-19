package shelly

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

type Gen2Meter struct {
	Client *http.Client
	Addr   string
}

type Gen2MeterData struct {
	// Sum of the active power on all phases.
	// Positive values is power taken from the grid/uplink.
	// Negative values is power injected to the grid/uplink.
	TotalPowerFloat float64 `json:"total_act_power"`
}

func (d Gen2MeterData) TotalPower() float64 {
	return d.TotalPowerFloat
}

// Read returns the whole Shelly3EMData status update from the Shelly 3EM.
func (s Gen2Meter) Read() (*Gen2MeterData, error) {
	url := url.URL{
		Scheme:   "http",
		Host:     s.Addr,
		Path:     "/rpc/EM.GetStatus",
		RawQuery: "id=0",
	}

	req, err := http.NewRequest(http.MethodGet, url.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to construct request: %w", err)
	}
	resp, err := s.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to read from shelly device: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code from shelly device: %v", resp.StatusCode)
	}

	// we expect no valid response larger than 1mb
	bodyReader := io.LimitReader(resp.Body, 1024*1024)

	data := new(Gen2MeterData)
	d := json.NewDecoder(bodyReader)
	err = d.Decode(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse response from shelly device: %w", err)
	}

	return data, nil
}
