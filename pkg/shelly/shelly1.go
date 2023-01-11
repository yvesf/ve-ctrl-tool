package shelly

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

type Shelly1 struct {
	Client *http.Client
	Addr   string
}

func (s *Shelly1) Set(state bool) error {
	url := url.URL{
		Scheme: "http",
		Host:   s.Addr,
		Path:   fmt.Sprintf("/relay/%v", 0),
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

	resp, err := s.Client.Do(req)
	if err != nil {
		return fmt.Errorf("shelly request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("shelly responded non 200 status-code %v", resp.StatusCode)
	}
	return nil
}

func (s *Shelly1) Get() (bool, error) {
	url := url.URL{
		Scheme: "http",
		Host:   s.Addr,
		Path:   fmt.Sprintf("/relay/%v", 0),
	}

	req, _ := http.NewRequest(http.MethodGet, url.String(), nil)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
	defer cancel()
	req = req.WithContext(ctx)

	resp, err := s.Client.Do(req)
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
