// forecast is a very simplified battery charging planner.
// The goal is the minimize the charging power to the lowest possible value that is sufficient per day.
// The biggest shortcut may be in just assuming that above a certain irradiation,
// there will be enough power generated to charge the battery, regardless of potential other local consumers.
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
	flagHTTP             = flag.String("http", "http://localhost:18001", "HTTP URL to the ve-ess-shelly endpoint.")
	flagLat              = flag.Float64("lat", 48.517737, "WGS-84 Latitude for fetching the forecast")
	flagLon              = flag.Float64("lon", 7.917704, "WGS-84 Longitude for fetching the forecast")
	flagChargeLimit      = flag.Float64("chargeLimit", 3, "Upper limit for maxCharge value [kW]")
	flagMiniumIrradiance = flag.Float64("minimumIrradiance", 150, "Threshold of forecasted [W/m²] above which the battery can be charge")
)

func main() {
	flag.Parse()
	cmd.SetupLogger()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	var err error
	for {
		var chargeAmountKWh float64
		for retry := range 10 {
			if retry > 0 {
				select {
				case <-ctx.Done():
					return
				case <-time.After(time.Second * time.Duration(retry)):
				}
			}
			chargeAmountKWh, err = fetchBatteryFreeCapacity()
			if err == nil {
				break
			}
			slog.Error("failed fetching battery capacity", slog.Any("err", err), slog.Int("retry", retry))
		}
		if err != nil {
			slog.Error("out of retries fetching battery capacity", slog.Any("err", err))
			return
		}
		slog.Info("battery capacity", slog.Float64("chargeAmountKWh", chargeAmountKWh))

		var chargingHours time.Duration
		for retry := range 10 {
			if retry > 0 {
				select {
				case <-ctx.Done():
					return
				case <-time.After(time.Second * time.Duration(retry)):
				}
			}
			chargingHours, err = fetchForecastChargingHours(time.Now())
			if err == nil {
				break
			}
			slog.Error("failed fetching forecast", slog.Any("err", err))
		}
		if err != nil {
			slog.Error("out of retries fetching forecast", slog.Any("err", err))
			return
		}
		slog.Info("forecast", slog.String("chargingHours", chargingHours.String()))

		chargeLimitKW := chargeAmountKWh / chargingHours.Hours()
		// add some safety margin (10%) to compensate losses and ensure battery will be fully loaded.
		chargeLimitKW = chargeLimitKW * 1.1
		if chargeLimitKW > *flagChargeLimit {
			chargeLimitKW = *flagChargeLimit
		}

		for retry := range 10 {
			if retry > 0 {
				select {
				case <-ctx.Done():
					return
				case <-time.After(time.Second * time.Duration(retry)):
				}
			}
			err = setChargeLimit(chargeLimitKW)
			if err == nil {
				break
			}
			slog.Error("failed set charge limit", slog.Any("err", err), slog.Int("retry", retry))
		}
		if err != nil {
			slog.Error("out of retries set charge limit", slog.Any("err", err))
			return
		}
		slog.Info("setChargeLimit", slog.Float64("chargeLimitKW", chargeLimitKW))

		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Minute * 10):
			continue
		}
	}
}

// setChargeLimit set's the charging power limitation on the controller.
func setChargeLimit(chargeLimitKW float64) error {
	data := url.Values{"maxCharge": []string{strconv.Itoa(int(chargeLimitKW * 1000))}}.Encode()
	resp, err := http.Post(*flagHTTP+"/flags", "application/x-www-form-urlencoded", strings.NewReader(data))
	if err != nil {
		return fmt.Errorf("POST controller /flags to set maxCharge: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("POST controller /flags response status code %d != 200", resp.StatusCode)
	}
	return nil
}

// fetchForecastChargingHours returns the expected duration where there is enough solar radiation to charge the battery.
func fetchForecastChargingHours(now time.Time) (time.Duration, error) {
	url, err := url.Parse("https://api.open-meteo.com/v1/forecast?minutely_15=global_tilted_irradiance_instant&timezone=UTC&forecast_days=3")
	if err != nil {
		return 0, err
	}
	q := url.Query()
	q.Add("latitude", strconv.FormatFloat(*flagLat, 'f', 6, 64))
	q.Add("longitude", strconv.FormatFloat(*flagLon, 'f', 6, 64))
	url.RawQuery = q.Encode()

	resp, err := http.Get(url.String())
	if err != nil {
		return 0, fmt.Errorf("GET open-meteo forecast: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("GET open-meteo forecast response status: %s", http.StatusText(resp.StatusCode))
	}

	forecastData := struct {
		Minutely15 struct {
			Time       []string  `json:"time"`
			Irradiance []float64 `json:"global_tilted_irradiance_instant"` // Unit: W/m²
		} `json:"minutely_15"`
	}{}
	err = json.NewDecoder(resp.Body).Decode(&forecastData)
	if err != nil {
		return 0, fmt.Errorf("parsing open-meteo response: %w", err)
	}

	var chargingHours time.Duration
	for i, datestamp := range forecastData.Minutely15.Time {
		// time.Parse uses UTC by default, and this matches the response with parameter timezone=UTC.
		parsed, err := time.Parse("2006-01-02T15:04", datestamp)
		if err != nil {
			return 0, err
		}
		// sum all sunshine time on the current calendar day after now.
		if parsed.Local().YearDay() == now.Local().YearDay() &&
			parsed.After(now.Add(-15*time.Minute)) {
			irradiance := forecastData.Minutely15.Irradiance[i]
			if irradiance > *flagMiniumIrradiance {
				chargingHours += time.Minute * time.Duration(15)
			}
		}
	}

	return chargingHours, nil
}

// fetchBatteryFreeCapacity returns the assumed battery capacity that can be charged up.
func fetchBatteryFreeCapacity() (float64, error) {
	resp, err := http.Get(*flagHTTP + "/status")
	if err != nil {
		return 0, fmt.Errorf("GET controller /status: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("GET controller /status response status: %s", http.StatusText(resp.StatusCode))
	}

	var responseBody struct {
		BatteryCapacityKWh float64 `json:"batteryCapacityKWh"`
		SoC                float64 `json:"soc"`
	}
	err = json.NewDecoder(resp.Body).Decode(&responseBody)
	if err != nil {
		return 0, err
	}

	// from the current SoC and the battery total capacity,
	// calculate the remaining free capacity to charge the battery.
	return (1.0 - responseBody.SoC/100.0) * responseBody.BatteryCapacityKWh, nil
}
