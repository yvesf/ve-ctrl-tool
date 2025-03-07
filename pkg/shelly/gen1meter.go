package shelly

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

type Gen1Meter struct {
	Client *http.Client
	Addr   string
}

type Gen1MeterData struct {
	// TotalPower is signed integer that reflects the total consumption of the measurement point.
	// Positive values is power taken from the grid/uplink.
	// Negative values is power injected to the grid/uplink.
	TotalPower_ float64 `json:"total_power"`
}

func (d Gen1MeterData) TotalPower() float64 {
	return d.TotalPower_
}

// Read returns the whole Shelly3EMData status update from the Shelly 3EM.
func (s Gen1Meter) Read() (*Gen1MeterData, error) {
	url := url.URL{
		Scheme: "http",
		Host:   s.Addr,
		Path:   "/status",
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

	data := new(Gen1MeterData)
	d := json.NewDecoder(bodyReader)
	err = d.Decode(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse response from shelly device: %w", err)
	}

	return data, nil
}
