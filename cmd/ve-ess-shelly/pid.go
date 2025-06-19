package main

import (
	"time"

	"github.com/bsm/openmetrics"
	"github.com/felixge/pidctrl"
)

var (
	metricControlSetpoint = openmetrics.DefaultRegistry().Gauge(openmetrics.Desc{
		Name: "ess_pid_setpoint",
		Unit: "watt",
		Help: "The current setpoint calculated by the PID controller",
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
)

type PIDWithMetrics struct {
	*pidctrl.PIDController
}

func NewPIDWithMetrics(p, i, d float64) PIDWithMetrics {
	return PIDWithMetrics{
		PIDController: pidctrl.NewPIDController(p, i, d),
	}
}

func (p PIDWithMetrics) SetOutputLimits(valueMin, valueMax float64) {
	p.PIDController.SetOutputLimits(valueMin, valueMax)
	metricControlPIDMin.With().Set(valueMin)
	metricControlPIDMax.With().Set(valueMax)
}

func (p PIDWithMetrics) UpdateDuration(value float64, duration time.Duration) float64 {
	out := p.PIDController.UpdateDuration(value, duration)
	metricControlSetpoint.With().Set(out)
	return out
}
