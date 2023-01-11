package main

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type MockEnergyMeter struct {
	ReadFn func() (PowerFlowWatt, error)
}

func (t *MockEnergyMeter) Read() (PowerFlowWatt, error) {
	return t.ReadFn()
}

var _ energyMeter = &MockEnergyMeter{}

type MockESSControl struct {
	SetpointSetFn func(ctx context.Context, value int16) error
	StatsFn       func(ctx context.Context) (EssStats, error)
}

func (e *MockESSControl) SetpointSet(ctx context.Context, value int16) error {
	return e.SetpointSetFn(ctx, value)
}

func (e *MockESSControl) Stats(ctx context.Context) (EssStats, error) {
	return e.StatsFn(ctx)
}

var _ essControl = &MockESSControl{}

type testCase struct {
	name             string
	initialPowerDraw float64
	watcher          func(*testing.T, <-chan PowerFlowWatt, context.CancelFunc)
}

// TestRunWithoutConsumer for "run" - testing concurrent appears to be complicated.
// this is am experimental best effort approach.
func TestRunWithoutConsumer(t *testing.T) {
	for _, tc := range []testCase{
		{
			name:             "production",
			initialPowerDraw: -200,
			watcher: func(t *testing.T, c <-chan PowerFlowWatt, cf context.CancelFunc) {
				for {
					pfw := <-c
					if pfw.ConsumptionPositive() < 0 && pfw.ConsumptionPositive() > -20 {
						cf()
						return
					}
				}
			},
		},
		{
			name:             "consumption",
			initialPowerDraw: 200,
			watcher: func(t *testing.T, c <-chan PowerFlowWatt, cf context.CancelFunc) {
				for {
					pfw := <-c
					if pfw.ConsumptionPositive() < 0 {
						cf()
						return
					}
				}
			},
		},
		{
			name:             "no-oscillation",
			initialPowerDraw: 100,
			watcher: func(t *testing.T, c <-chan PowerFlowWatt, cf context.CancelFunc) {
				for {
					pfw := <-c
					if pfw.ConsumptionPositive() < 0 {
						break
					}
				}

				var production, consumption float64
				var lastMeasurement time.Time
				start := time.Now()
				for time.Since(start) < time.Second*10 {
					pfw := (<-c).ConsumptionNegative()
					if !lastMeasurement.IsZero() {
						amount := float64(time.Since(lastMeasurement).Milliseconds()) * pfw
						if amount < 0 {
							consumption += amount
						} else {
							production += amount
						}
					}
					lastMeasurement = time.Now()
				}
				cf()

				assert.InDelta(t, float64(0), consumption/1000, 0.0, "kilowatt/second")
				assert.InDelta(t, float64(0), production/1000, 150.0, "kilowatt/second")
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) { runTest(t, tc) })
	}
}

func runTest(t *testing.T, tc testCase) {
	var (
		mockEnergyMeter          MockEnergyMeter
		mockESSControl           MockESSControl
		setpoint                 float64
		setpointHardwareFeedback float64
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	watcherChan := make(chan PowerFlowWatt)
	defer close(watcherChan)

	go tc.watcher(t, watcherChan, cancel)

	go func() {
		for {
			previousSetpoint := setpoint
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Millisecond * 1500): // 1500 simulates the delay of the inverter
				setpointHardwareFeedback = previousSetpoint
			}
		}
	}()

	mockEnergyMeter.ReadFn = func() (pfw PowerFlowWatt, err error) {
		currentEnergyReading := ConsumptionPositive(tc.initialPowerDraw - setpointHardwareFeedback)
		select {
		case watcherChan <- currentEnergyReading:
		default:
		}
		t.Logf("Return energy reading %v", currentEnergyReading)
		return currentEnergyReading, nil
	}

	mockESSControl.SetpointSetFn = func(callCtx context.Context, value int16) error {
		if errors.Is(ctx.Err(), context.Canceled) && value == 0 {
			// final reset to 0
			t.Logf("Reset Inverter to %v (sleep 200ms)", setpoint)
		} else {
			t.Logf("SetpointSet %v (sleep 200ms)", setpoint)
			setpoint = float64(value)
		}
		time.Sleep(time.Millisecond * 200)
		return nil
	}

	mockESSControl.StatsFn = func(ctx context.Context) (EssStats, error) {
		return EssStats{
			IBat:          12,
			UBat:          12,
			InverterPower: int(setpoint),
		}, nil
	}

	*flagMaxWattInverter = 300

	run(ctx, &mockESSControl, &mockEnergyMeter, nil)

	require.InDelta(t, tc.initialPowerDraw, setpoint, math.Abs(*flagOffset)+setpointRounding)
}
