package shelly

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

type Relay struct {
	Client *http.Client
	Addr   string
	Number int
}

func (r Relay) Set(state bool) error {
	url := url.URL{
		Scheme: "http",
		Host:   r.Addr,
		Path:   fmt.Sprintf("/relay/%v", r.Number),
	}
	if state {
		url.RawQuery = "turn=on"
	} else {
		url.RawQuery = "turn=off"
	}

	req, _ := http.NewRequest(http.MethodGet, url.String(), nil)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
	defer cancel()
	req = req.WithContext(ctx)

	resp, err := r.Client.Do(req)
	if err != nil {
		return fmt.Errorf("shelly request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("shelly responded non 200 status-code %v", resp.StatusCode)
	}
	return nil
}

func (r Relay) Get() (bool, error) {
	url := url.URL{
		Scheme: "http",
		Host:   r.Addr,
		Path:   fmt.Sprintf("/relay/%v", 0),
	}

	req, _ := http.NewRequest(http.MethodGet, url.String(), nil)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
	defer cancel()
	req = req.WithContext(ctx)

	resp, err := r.Client.Do(req)
	if err != nil {
		return false, fmt.Errorf("shelly request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("shelly responded non 200 status-code %v", resp.StatusCode)
	}

	doc := struct {
		Ison bool `json:"ison"`
	}{}
	err = json.NewDecoder(resp.Body).Decode(&doc)
	if err != nil {
		return false, fmt.Errorf("failed to decode response: %w", err)
	}

	return doc.Ison, nil
}

func MakeMeteredRelay() {
}

func (r Relay) WithMeter() MeterRelay {
	return MeterRelay{
		Relay: r,
		Meter: Meter{
			Client: r.Client,
			Addr:   r.Addr,
		},
	}
}
