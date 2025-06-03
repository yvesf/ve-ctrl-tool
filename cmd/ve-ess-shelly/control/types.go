package control

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// PowerFlowWatt type represent power that can flow in two directions: Production and Consumption
// Flow is represented by positive/negative values.
type PowerFlowWatt float64

func ConsumptionPositive(watt float64) PowerFlowWatt {
	return PowerFlowWatt(watt)
}

func (p PowerFlowWatt) String() string {
	if p < 0 {
		return fmt.Sprintf("Production(%.2f)", -1*p)
	}
	return fmt.Sprintf("Consumption(%.2f)", p)
}

func (p PowerFlowWatt) ConsumptionPositive() float64 {
	return float64(p)
}

func (p PowerFlowWatt) ConsumptionNegative() float64 {
	return float64(-p)
}

// Measurement is to share PowerFlowWatt values between go-routines.
// like an "Optional" type it can set to hold currently no value.
type Measurement struct {
	m     sync.RWMutex
	value PowerFlowWatt
	valid bool
}

func (o *Measurement) SetInvalid() {
	o.m.Lock()
	defer o.m.Unlock()
	o.valid = false
	o.value = 0.0
}

func (o *Measurement) Set(v PowerFlowWatt) {
	o.m.Lock()
	defer o.m.Unlock()
	o.value = v
	o.valid = true
}

func (o *Measurement) Get() (value PowerFlowWatt, valid bool) {
	o.m.RLock()
	defer o.m.RUnlock()
	if !o.valid {
		return 0.0, false
	}
	return o.value, true
}

type EssStats struct {
	IBat, UBat    float64
	InverterPower int
}

type ESSControl interface {
	Stats(ctx context.Context) (EssStats, error)
	SetpointSet(ctx context.Context, value int16) error
}

type EnergyMeter interface {
	LastMeasurement() (PowerFlowWatt, time.Time)
}

type Settings struct {
	// MaxWattCharge [Watt] is the maximum power to charge the battery (negative ESS setpoint).
	MaxWattCharge int
	// MaxWattInverter [Watt] is the maximum power to generate (positive ESS setpoint).
	MaxWattInverter int
	// PowerOffset [Watt] is a constant offset applied to the metered power flow.
	PowerOffset int
	// SetpointRounding [Watt] is applied on the calculated setpoint to also lower the amount of ESS Communication.
	SetpointRounding int
	// ZeroPointWindow [Watt] is a power window around zero in which no change is applied to lower the
	// amount of ESS communication.
	ZeroPointWindow int
}
