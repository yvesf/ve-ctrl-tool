package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/yvesf/ve-ctrl-tool/cmd"
	"github.com/yvesf/ve-ctrl-tool/cmd/ve-ess-shelly/control"
	"github.com/yvesf/ve-ctrl-tool/pkg/mk2"
	"github.com/yvesf/ve-ctrl-tool/pkg/shelly"
	"golang.org/x/exp/slog"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	var settings control.Settings
	flag.IntVar(&settings.MaxWattCharge, "maxCharge", 250.0,
		"Maximum ESS Setpoint for charging (negative setpoint)")
	flag.IntVar(&settings.MaxWattInverter, "maxInverter", 60.0,
		"Maximum ESS Setpoint for inverter (positive setpoint)")
	flag.IntVar(&settings.MaxWattInverterPeak, "maxInverterPeak", 800,
		"Maximum ESS Setpoint for inverter (positive setpoint) for peaks after recent peak charging phase")
	flag.IntVar(&settings.PowerOffset, "offset", -4.0,
		"Power measurement offset")
	flag.IntVar(&settings.SetpointRounding, "setpointRounding", 5.0,
		"Round setpoint to this step")
	flag.IntVar(&settings.ZeroPointWindow, "zeroWindow", 10.0,
		"Do not operate if measurement is in this +/- window")

	adapter := cmd.CommonInit(ctx)

	mk2Ess, err := mk2.ESSInit(ctx, adapter)
	if err != nil {
		panic(err)
	}

	shelly := shelly.Meter{Addr: flag.Args()[0], Client: http.DefaultClient}
	m := &meter{Meter: shelly}

	var meterError error
	go func() {
		meterError = m.Run(ctx)
		cancel()
	}()

	err = control.Run(ctx, settings, inverter{adapter: mk2Ess}, m)
	if err != nil && err != context.Canceled {
		slog.Error("run failed", slog.Any("err", err))
		os.Exit(1)
	}

	if meterError != nil {
		slog.Error("reading from meter failed", slog.Any("err", err))
		os.Exit(1)
	}
}
