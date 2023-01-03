// gpio provides a RPi-GPIO connected consumer to switch on/off depending on
// available power.

package gpio

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/bsm/openmetrics"
	"github.com/rs/zerolog/log"
	"github.com/warthog618/gpiod"
)

var metricGpioPower = openmetrics.DefaultRegistry().Gauge(openmetrics.Desc{
	Name:   "ess_gpio_power",
	Unit:   "watt",
	Help:   "power allocated to gpio-connected devices",
	Labels: []string{"gpio"},
})

type Consumer struct {
	// configuration:
	gpio      *gpiod.Line
	delay     time.Duration
	powerWatt int
	// state
	lastOn           time.Time
	lastOnCondition  time.Time
	lastOffCondition time.Time
	isEnabled        bool // true=on
}

// Offer gives the GPIO choice to switch on/off.
// Returns true if state was changed. This indication can be used to backoff from further changes to other GPIOs.
func (g *Consumer) Offer(powerWatt int) bool {
	// IF powerWatt is positive: we are consuming energy - consider to switch off
	if powerWatt > 0 {
		g.lastOnCondition = time.Time{}
		if g.lastOffCondition.IsZero() {
			g.lastOffCondition = time.Now()
		}
		// IF ENABLED and SINCE(lastOn) > DELAY
		if g.isEnabled &&
			time.Since(g.lastOn) > g.delay && // minimal on-time
			time.Since(g.lastOffCondition) > g.delay {
			g.lastOn = time.Time{}
			g.isEnabled = false
			g.writeGpio()
			return true
		}
		// ELSE IF powerWatt is negative: we have a surplus energy - consider to switch on
	} else {
		// IF powerWatt surplus > powerWatt
		if powerWatt*-1 > g.powerWatt {
			g.lastOffCondition = time.Time{}
			if g.lastOnCondition.IsZero() {
				g.lastOnCondition = time.Now()
			}
			if !g.isEnabled && time.Since(g.lastOnCondition) > g.delay {
				g.lastOn = time.Now()
				g.isEnabled = true
				g.writeGpio()
				return true
			}
		}
	}
	return false
}

func (g *Consumer) writeGpio() {
	log.Info().Int("gpio", g.gpio.Offset()).Int("powerWatt", g.powerWatt).Bool("isEnabled", g.isEnabled).
		Time("lastOnCondition", g.lastOnCondition).Time("laston", g.lastOn).Time("lastOffCondition", g.lastOffCondition).
		Msg("Write GPIO")
	if g.isEnabled {
		metricGpioPower.With(strconv.Itoa(g.gpio.Offset())).Set(float64(g.powerWatt))
		err := g.gpio.SetValue(1)
		if err != nil {
			log.Error().Err(err).Msg("failed to raise gpio")
		}
	} else {
		metricGpioPower.With(strconv.Itoa(g.gpio.Offset())).Set(0.0)
		err := g.gpio.SetValue(0)
		if err != nil {
			log.Error().Err(err).Msg("failed to lower gpio")
		}
	}
}

type ConsumerList []*Consumer

// String provides a to-string formatting in conformance with the `flags` package.
func (i *ConsumerList) String() string {
	var els []string
	for _, c := range *i {
		els = append(els, fmt.Sprintf("Consumer(gpio-%v, %v W, delay %vs)",
			c.gpio.Offset(), c.powerWatt, c.delay.Seconds()))
	}
	return strings.Join(els, ", ")
}

// Set provides string-parsing in conformance with the `flags` package.
// Calling Set parses 'value' and adds it as GpioConsumer to the list 'i'.
func (i *ConsumerList) Set(value string) error {
	v := strings.Split(value, `,`)
	if len(v) != 3 {
		return fmt.Errorf("invalid number of parameters for external consumer")
	}

	gpio, err := strconv.ParseInt(v[0], 10, 32)
	if err != nil {
		return fmt.Errorf("failed to parse gpio parameter: %w", err)
	}

	power, err := strconv.ParseInt(v[1], 10, 32)
	if err != nil {
		return fmt.Errorf("failed to parse watt parameter: %w", err)
	}

	delay, err := time.ParseDuration(v[2])
	if err != nil {
		return fmt.Errorf("failed to parse delay parameter: %w", err)
	}

	pin, err := gpiod.RequestLine("gpiochip0", int(gpio), gpiod.AsOutput(0))
	if err != nil {
		return fmt.Errorf("failed to initialize gpio %v: %w", gpio, err)
	}
	GpioConsumer := &Consumer{
		gpio:      pin,
		powerWatt: int(power),
		delay:     delay,
		isEnabled: false,
	}
	GpioConsumer.writeGpio() // initialize gpio output

	*i = append(*i, GpioConsumer)
	return nil
}

// LastChange returns the most recent change in the list of gpios
// or zero-time if none have changed ever.
func (i *ConsumerList) LastChange() time.Time {
	var max time.Time
	for _, g := range *i {
		if !g.lastOn.IsZero() && (max.IsZero() || g.lastOn.After(max)) {
			max = g.lastOn
		}
	}
	return max
}

func (i *ConsumerList) Close() {
	for _, g := range *i {
		_ = g.gpio.SetValue(0)
		_ = g.gpio.Close()
	}
}
