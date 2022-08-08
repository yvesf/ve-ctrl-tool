package meter

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type Shelly3EM struct {
	url string
}

func NewShelly3EM(url string) *Shelly3EM {
	return &Shelly3EM{url: url}
}

type Shelly3EMData struct {
	EMeters []struct {
		Current       float64 `json:"current"`
		IsValid       bool    `json:"is_valid"`
		PowerFactor   float64 `json:"pf"`
		Power         float64 `json:"power"`
		Total         float64 `json:"total"`
		TotalReturned float64 `json:"total_returned"`
		Voltage       float64 `json:"voltage"`
	} `json:"emeters"`
	FsFree    int    `json:"fs_free"`
	FsMounted bool   `json:"fs_mounted"`
	FsSize    int    `json:"fs_size"`
	HasUpdate bool   `json:"has_update"`
	MAC       string `json:"mac"`
	MQTT      struct {
		Connected bool `json:"connected"`
	} `json:"mqtt"`
	RamFree  int `json:"ram_free"`
	RamTotal int `json:"ram_total"`
	Relays   []struct {
		HasTimer       bool   `json:"has_timer"`
		IsValid        bool   `json:"is_valid"`
		IsOn           bool   `json:"ison"`
		Overpower      bool   `json:"overpower"`
		Source         string `json:"source"`
		TimerDuration  int    `json:"timer_duration"`
		TimerRemaining int    `json:"timer_remaining"`
		TimerStarted   int    `json:"timer_started"`
	} `json:"relays"`
	Serial     int     `json:"serial"`
	Time       string  `json:"time"`
	TotalPower float64 `json:"total_power"`
	Unixtime   int     `json:"unixtime"`
	Update     struct {
		HasUpdate  bool   `json:"has_update"`
		NewVersion string `json:"new_version"`
		OldVersion string `json:"old_version"`
		Status     string `json:"status"`
	} `json:"update"`
	Uptime  int `json:"uptime"`
	WifiSta struct {
		Connected bool   `json:"connected"`
		Ip        string `json:"ip"`
		Rssi      int    `json:"rssi"`
		Ssid      string `json:"ssid"`
	} `json:"wifi_sta"`
}

func (s Shelly3EM) Read() (*Shelly3EMData, error) {
	resp, err := http.Get(s.url + `/status`)
	if err != nil {
		return nil, fmt.Errorf("failed to read from shelly device: %w", err)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("unexpected status code from shelly device: %v", resp.StatusCode)
	}

	var data = new(Shelly3EMData)
	d := json.NewDecoder(resp.Body)
	err = d.Decode(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse response from shelly device: %w", err)
	}

	return data, nil
}

// ReadTotalPower returns a signed integer that reflects the total consumption of the measurement point.
// Positive values is power taken from the grid/uplink.
// Negative values is power injected to the grid/uplink.
func (s Shelly3EM) ReadTotalPower() (float64, error) {
	d, err := s.Read()
	if err != nil {
		return 0.0, err
	}
	return d.TotalPower, nil
}
