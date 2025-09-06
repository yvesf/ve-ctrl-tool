package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/bsm/openmetrics"
	"github.com/bsm/openmetrics/omhttp"
	"github.com/yvesf/ve-ctrl-tool/cmd"
	"github.com/yvesf/ve-ctrl-tool/pkg/httpflag"
	"github.com/yvesf/ve-ctrl-tool/pkg/mk2"
	"github.com/yvesf/ve-ctrl-tool/pkg/shelly"
)

var (
	// MaxWattCharge [Watt] is the maximum power to charge the battery (negative ESS setpoint).
	SettingsMaxWattCharge = flag.Int("maxCharge", 250.0, "Maximum ESS Setpoint for charging (negative setpoint)")
	// MaxWattInverter [Watt] is the maximum power to generate (positive ESS setpoint).
	SettingsMaxWattInverter = flag.Int("maxInverter", 60.0, "Maximum ESS Setpoint for inverter (positive setpoint)")
	// PowerOffset [Watt] is a constant offset applied to the metered power flow.
	SettingsPowerOffset = flag.Int("offset", -4.0, "Power measurement offset")
	// SetpointRounding [Watt] is applied on the calculated setpoint to also lower the amount of ESS Communication.
	SettingsSetpointRounding = flag.Int("setpointRounding", 3.0, "Round setpoint to this step")
	// ZeroPointWindow [Watt] is a power window around zero in which no change is applied to lower the
	// amount of ESS communication.
	SettingsZeroPointWindow = flag.Int("zeroWindow", 10.0, "Do not operate if measurement is in this +/- window")
	// SettingHTTPAddr configures the configuration HTTP server address to listen.
	SettingsHTTP = flag.String("http", "", "start http server for metrics and flags")
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	flag.Parse()

	var statusHandler http.HandlerFunc
	if *SettingsHTTP != `` {
		mux := http.NewServeMux()
		mux.Handle("/metrics", omhttp.NewHandler(openmetrics.DefaultRegistry()))
		mux.Handle("/flags", httpflag.NewHandler(flag.VisitAll))
		mux.Handle("/status", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if statusHandler == nil {
				http.Error(w, "not ready", http.StatusInternalServerError)
				return
			}
			statusHandler(w, r)
		}))

		srv := &http.Server{
			Handler: mux,
			Addr:    *SettingsHTTP,
		}
		go func() {
			err := srv.ListenAndServe()
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				slog.Error("http server failed", slog.Any("err", err))
				os.Exit(1)
			}
		}()
	}

	cmd.SetupLogger()
	adapter := cmd.InitMk2(ctx)

	mk2Ess, err := mk2.ESSInit(ctx, adapter)
	if err != nil {
		panic(err)
	}

	shelly := shelly.Gen2Meter{Addr: flag.Args()[0], Client: http.DefaultClient}
	m := &meterReader{Meter: shelly}

	var meterError error
	go func() {
		meterError = m.Run(ctx)
		cancel()
	}()

	c := Controller{
		meter: m,
		ess:   mk2Ess,
	}
	statusHandler = c.ServeHTTP

	err = c.Run(ctx)
	if err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("run failed", slog.Any("err", err))
		os.Exit(1)
	}

	if meterError != nil {
		slog.Error("reading from meter failed", slog.Any("err", err))
		os.Exit(1)
	}
}
