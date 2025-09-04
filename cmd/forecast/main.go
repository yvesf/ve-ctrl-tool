package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"github.com/yvesf/ve-ctrl-tool/cmd"
)

var (
	flagHTTPServer          = flag.String("http", "http://localhost:18001/flags", "HTTP url to the flags endpoint.")
	flagLat                 = flag.Float64("lat", 48.517737, "Latitude")
	flagLon                 = flag.Float64("lon", 7.917704, "Longitude")
	flagBatteryFullCapacity = flag.Float64("capacity", 5, "Battery daily target charging amount [kWh]")
)

func main() {
	flag.Parse()
	cmd.SetupLogger()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	for {
		sd, err := fetchSunshineForecast(time.Now())
		if err != nil {
			slog.Error("failed fetching forecast", slog.Any("err", err))
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Minute * 10):
				continue
			}
		}

		slog.Info("forecast", slog.String("sunshine", sd.Round(time.Minute).String()))

		err = setChargeLimit(sd)
		if err != nil {
			slog.Error("failed set charge limit", slog.Any("err", err))
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Minute * 10):
				continue
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Minute * 10):
			continue
		}
	}
}

func setChargeLimit(sunshineDuration time.Duration) error {
	chargeKW := 1000 * (*flagBatteryFullCapacity / sunshineDuration.Hours())
	if chargeKW > 2500 {
		chargeKW = 2500
	}

	data := fmt.Sprintf("maxCharge=%d", int(chargeKW))
	slog.Info("set chargeLimit", slog.String("request", data))

	resp, err := http.Post(*flagHTTPServer, "application/x-www-form-urlencoded", strings.NewReader(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("response status code %d != 200", resp.StatusCode)
	}

	return nil
}

func fetchSunshineForecast(t time.Time) (time.Duration, error) {
	url, err := url.Parse("https://api.open-meteo.com/v1/forecast?daily=sunshine_duration&timezone=Europe%2FBerlin&forecast_days=3")
	if err != nil {
		return 0, err
	}
	q := url.Query()
	q.Add("latitude", strconv.FormatFloat(*flagLat, 'f', 6, 64))
	q.Add("longitude", strconv.FormatFloat(*flagLon, 'f', 6, 64))
	url.RawQuery = q.Encode()

	resp, err := http.Get(url.String())
	if err != nil {
		return 0, err
	}
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("response status code %d != 200", resp.StatusCode)
	}

	result := struct {
		Daily struct {
			Time             []string  `json:"time"`
			SunshineDuration []float64 `json:"sunshine_duration"`
		} `json:"daily"`
	}{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	if err != nil {
		return 0, err
	}

	for i, datestamp := range result.Daily.Time {
		parsed, err := time.Parse(time.DateOnly, datestamp)
		if err != nil {
			return 0, err
		}
		if parsed.Local().YearDay() == t.Local().YearDay() {
			duration := result.Daily.SunshineDuration[i] * float64(time.Second)
			return time.Duration(duration), nil
		}
	}

	return 0, fmt.Errorf("no forecast found for %s", t.String())
}
