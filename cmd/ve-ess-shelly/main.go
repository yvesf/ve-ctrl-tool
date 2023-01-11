package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/yvesf/ve-ctrl-tool/cmd"
	"github.com/yvesf/ve-ctrl-tool/consumer"
	"github.com/yvesf/ve-ctrl-tool/mk2"
	"github.com/yvesf/ve-ctrl-tool/pkg/shelly"
	"github.com/yvesf/ve-ctrl-tool/pkg/vebus"

	"github.com/bsm/openmetrics"
	"github.com/bsm/openmetrics/omhttp"
	"github.com/rs/zerolog/log"
)

var (
	metricControlSetpoint = openmetrics.DefaultRegistry().Gauge(openmetrics.Desc{
		Name: "ess_pid_setpoint",
		Unit: "watt",
		Help: "The current setpoint calculated by the PID controller",
	})
	metricControlInput = openmetrics.DefaultRegistry().Gauge(openmetrics.Desc{
		Name: "ess_pid_input",
		Unit: "watt",
		Help: "The current input for the PID controller",
	})
	metricControlInputDiff = openmetrics.DefaultRegistry().Gauge(openmetrics.Desc{
		Name: "ess_pid_input_diff",
		Unit: "watt",
		Help: "The current input difference for stabilization of the PID controller",
	})
	metricControlPIDMin = openmetrics.DefaultRegistry().Gauge(openmetrics.Desc{
		Name: "ess_pid_output_min",
		Unit: "watt",
		Help: "PID min",
	})
	metricControlPIDMax = openmetrics.DefaultRegistry().Gauge(openmetrics.Desc{
		Name: "ess_pid_output_max",
		Unit: "watt",
		Help: "PID max",
	})
	metricMultiplusSetpoint = openmetrics.DefaultRegistry().Gauge(openmetrics.Desc{
		Name: "ess_multiplus_setpoint",
		Unit: "watt",
		Help: "The setpoint written to the multiplus",
	})
	metricMultiplusIBat = openmetrics.DefaultRegistry().Gauge(openmetrics.Desc{
		Name: "ess_multiplus_ibat",
		Unit: "ampere",
		Help: "Current of the multiplus battery, negative=discharge",
	})
	metricMultiplusUBat = openmetrics.DefaultRegistry().Gauge(openmetrics.Desc{
		Name: "ess_multiplus_ubat",
		Unit: "voltage",
		Help: "Voltage of the multiplus battery",
	})
	metricMultiplusInverterPower = openmetrics.DefaultRegistry().Gauge(openmetrics.Desc{
		Name: "ess_multiplus_inverter_power",
		Unit: "watt",
		Help: "Ram InverterPower1",
	})
	metricShellyPower = openmetrics.DefaultRegistry().Gauge(openmetrics.Desc{
		Name:   "ess_shelly_power",
		Unit:   "watt",
		Help:   "Power readings from shelly device",
		Labels: []string{"meter"},
	})
)

const (
	inverterPeakMaximumTimeWindow = time.Minute * 15 // in the last 15min.. for max 15min
)

var (
	flagMetricsHTTP     = flag.String("metricsHTTP", "", "Address of a http server serving metrics under /metrics")
	flagMaxWattCharge   = flag.Float64("maxCharge", 250.0, "Maximum ESS Setpoint for charging (negative setpoint)")
	flagMaxWattInverter = flag.Float64("maxInverter",
		60.0,
		"Maximum ESS Setpoint for inverter (positive setpoint)")
	flagMaxWattInverterPeak = flag.Float64("maxInverterPeak",
		800,
		"Maximum ESS Setpoint for inverter (positive setpoint) for peaks after recent peak charging phase")
	flagOffset          = flag.Float64("offset", -10.0, "Power measurement offset")
	flagZeroPointWindow = flag.Float64("zeroWindow", 20, "Do not operate if measurement is in this +/- window")
	flagConsumerDelay   = flag.Float64("consumerDelay", 30.0, "time in seconds between (de-)activating another consumer")
	flagConsumer        consumer.List
)

func main() {
	flag.Var(&flagConsumer, "consumer", "Consumers switches (repeat in priority order). "+
		"Format: POWER,DELAY,URL. Example: \"400,15m,shelly1://192.168.0.3\"")

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	adapter := cmd.CommonInit(ctx)

	log.Info().Msg("consumer switches " + flagConsumer.String())
	// reset consumers to 0
	for _, c := range flagConsumer {
		if err := c.Offer(0); err != nil {
			log.Info().Err(err).Msg("failed to initialize consumer")
		}
	}
	defer flagConsumer.Close()

	// Metrics HTTP endpoint
	if *flagMetricsHTTP != `` {
		mux := http.NewServeMux()
		mux.Handle("/metrics", omhttp.NewHandler(openmetrics.DefaultRegistry()))

		var lc net.ListenConfig
		ln, err := lc.Listen(ctx, "tcp", *flagMetricsHTTP)
		if err != nil {
			log.Panic().Err(err).Str("addr", *flagMetricsHTTP).Msg("Listen on http failed")
		}

		srv := &http.Server{Handler: mux}
		go func() {
			err := srv.Serve(ln)
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Error().Err(err).Msg("http server failed")
			}
		}()
	}

	mk2Ess, err := mk2.ESSInit(ctx, adapter)
	if err != nil {
		panic(err)
	}

	shelly := shelly.Shelly3EM{URL: flag.Args()[0], Client: http.DefaultClient}

	run(ctx, mk2Driver{mk2: adapter, ess: mk2Ess}, shellyDriver(shelly), flagConsumer)
}

type shellyDriver shelly.Shelly3EM

func (s shellyDriver) Read() (PowerFlowWatt, error) {
	data, err := shelly.Shelly3EM(s).Read()
	if err != nil {
		return 0, err
	}

	metricShellyPower.With("total").Set(data.TotalPower)
	for i, m := range data.EMeters {
		metricShellyPower.With(fmt.Sprintf("emeter%d", i)).Set(m.Power)
	}

	return ConsumptionPositive(data.TotalPower), nil
}

type mk2Driver struct {
	ess *mk2.ESS
	mk2 *mk2.Adapter
}

func (m mk2Driver) SetpointSet(ctx context.Context, value int16) error {
	return m.ess.SetpointSet(ctx, value)
}

func (m mk2Driver) Stats(ctx context.Context) (EssStats, error) {
	iBat, uBat, err := m.mk2.CommandReadRAMVarSigned16(ctx, vebus.RAMIDIBat, vebus.RAMIDUBat)
	if err != nil {
		return EssStats{}, fmt.Errorf("failed to read IBat/UBat: %w", err)
	}

	inverterPowerRAM, _, err := m.mk2.CommandReadRAMVarSigned16(ctx, vebus.RAMIDInverterPower1, 0)
	if err != nil {
		return EssStats{}, fmt.Errorf("failed to read InverterPower1: %w", err)
	}

	log.Debug().Float32("IBat", float32(iBat)/10).
		Float32("UBat", float32(uBat)/100).
		Float32("InverterPower", float32(inverterPowerRAM)).
		Msg("Multiplus Stats")
	metricMultiplusIBat.With().Set(float64(iBat) / 10)
	metricMultiplusUBat.With().Set(float64(uBat) / 100)
	metricMultiplusInverterPower.With().Set(float64(inverterPowerRAM))

	return EssStats{
		IBat:          float64(iBat) / 10,
		UBat:          float64(uBat) / 100,
		InverterPower: int(inverterPowerRAM),
	}, nil
}
