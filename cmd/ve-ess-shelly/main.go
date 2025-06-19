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

	"github.com/yvesf/ve-ctrl-tool/cmd"
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
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	adapter := cmd.CommonInit(ctx)

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

	err = RunController(ctx, &inverter{adapter: mk2Ess}, m)
	if err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("run failed", slog.Any("err", err))
		os.Exit(1)
	}

	if meterError != nil {
		slog.Error("reading from meter failed", slog.Any("err", err))
		os.Exit(1)
	}
}
