// consumer package for switching externally connected consumers depending on available power.

package consumer

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/bsm/openmetrics"
	"github.com/rs/zerolog/log"
	"github.com/yvesf/ve-ctrl-tool/pkg/shelly"
)

var metricEssConsumer = openmetrics.DefaultRegistry().Gauge(openmetrics.Desc{
	Name:   "ess_consumer_switch",
	Unit:   "state",
	Help:   "consumer state",
	Labels: []string{"id"},
})

var now = time.Now

type tristate int

func (t tristate) Bool() (value, ok bool) {
	switch t {
	case 1:
		return true, true
	case 2:
		return false, true
	default:
		return false, false
	}
}

func (t *tristate) Set(v bool) {
	if v {
		*t = 1
	} else {
		*t = 2
	}
}

type generic struct {
	// configuration:
	powerWatt int
	delay     time.Duration
	// state:
	lastOn           time.Time
	lastOnCondition  time.Time
	lastOff          time.Time
	lastOffCondition time.Time
	isEnabled        bool      // true=on
	backoffUntil     time.Time // if not zero, disable actions until this time
}

// Offer gives the consumer choice to switch on/off.
// Returns true if state was changed. This indication can be used to backoff from further changes to other consumers.
// If true was returned, Update needs to be called too.
func (g *generic) Offer(powerWatt int) error {
	now := now()
	if g.backoff() {
		return nil
	}
	// IF powerWatt is positive: we are consuming energy - consider to switch off
	if powerWatt > 0 {
		g.lastOnCondition = time.Time{}
		if g.lastOffCondition.IsZero() {
			g.lastOffCondition = now
		}
		// IF ENABLED and SINCE(lastOn) > DELAY
		if g.isEnabled &&
			now.Sub(g.lastOn) > g.delay && // minimal on-time
			now.Sub(g.lastOffCondition) > g.delay {
			g.lastOn = time.Time{}
			g.lastOff = now
			g.isEnabled = false
			return nil
		}
		// ELSE IF powerWatt is negative: we have a surplus energy - consider to switch on
	} else {
		// IF powerWatt surplus > powerWatt
		if powerWatt*-1 > g.powerWatt {
			g.lastOffCondition = time.Time{}
			if g.lastOnCondition.IsZero() {
				g.lastOnCondition = now
			}
			if !g.isEnabled && now.Sub(g.lastOnCondition) > g.delay {
				g.lastOn = now
				g.lastOff = time.Time{}
				g.isEnabled = true
				return nil
			}
		}
	}
	return nil
}

func (g *generic) backoff() bool {
	if g.backoffUntil.IsZero() {
		return false
	}
	if g.backoffUntil.Sub(now()) < 0 {
		g.backoffUntil = time.Time{}
		return false
	}
	return true
}

func (g *generic) LastChange() time.Time {
	if g.lastOn.After(g.lastOff) {
		return g.lastOn
	}
	return g.lastOff
}

func (g *generic) Close() {
	panic("not implemented")
}

type ShellyRelay struct {
	generic
	shelly.Shelly1
	// relayState holds last known state of the relay (true=on, false=off).
	relayState tristate
}

func (g *ShellyRelay) Offer(powerWatt int) error {
	if g.backoff() {
		return nil
	}

	_ = g.generic.Offer(powerWatt)

	logger := log.With().Str("shelly", g.Shelly1.Addr).Int("powerWatt", g.powerWatt).Bool("isEnabled", g.isEnabled).
		Time("lastOnCondition", g.lastOnCondition).Time("laston", g.lastOn).
		Time("lastOffCondition", g.lastOffCondition).Logger()
	if relayState, ok := g.relayState.Bool(); !ok || (ok && g.isEnabled != relayState) {
		logger.Info().Msg("Update")
		err := g.Shelly1.Set(g.isEnabled)
		if err != nil {
			logger.Error().Err(err).Msg("update failed, backoff 30s")
			g.backoffUntil = now().Add(time.Second * 30)
			return fmt.Errorf("shelly request failed: %w", err)
		}
		g.relayState.Set(g.isEnabled)
		if g.isEnabled {
			metricEssConsumer.With(g.Shelly1.Addr).Set(1)
		} else {
			metricEssConsumer.With(g.Shelly1.Addr).Set(0)
		}
	}
	return nil
}

func (g *ShellyRelay) String() string {
	return fmt.Sprintf("[%vW %vs shelly1://%v]", g.powerWatt, g.delay.Seconds(), g.Shelly1.Addr)
}

func (g *ShellyRelay) Close() error {
	return g.Shelly1.Set(false)
}

type Consumer interface {
	fmt.Stringer
	Offer(powerWatt int) error
	LastChange() time.Time
	Close() error
}

func Parse(s string) (Consumer, error) {
	v := strings.SplitN(s, `,`, 3)
	if len(v) != 3 {
		return nil, fmt.Errorf("invalid number of parameters for external consumer")
	}

	power, err := strconv.ParseInt(v[0], 10, 32)
	if err != nil {
		return nil, fmt.Errorf("failed to parse watt parameter: %w", err)
	}

	if power < 0 {
		return nil, fmt.Errorf("invalid power parameter")
	}

	delay, err := time.ParseDuration(v[1])
	if err != nil {
		return nil, fmt.Errorf("failed to parse delay parameter: %w", err)
	}

	if delay < 0 {
		return nil, fmt.Errorf("invalid delay parameter")
	}

	url, err := url.Parse(v[2])
	if err != nil {
		return nil, fmt.Errorf("failed to parse URL: %w", err)
	}

	generic := generic{
		powerWatt: int(power),
		delay:     delay,
	}

	switch url.Scheme {
	case "shelly1":
		if url.Host == `` {
			return nil, fmt.Errorf("empty host for shelly1")
		}
		return &ShellyRelay{
			generic: generic,
			Shelly1: shelly.Shelly1{
				Addr:   url.Host,
				Client: http.DefaultClient,
			},
		}, nil
	default:
		return nil, fmt.Errorf("unsupported scheme: %v", url.Scheme)
	}
}

type List []Consumer

// String provides a to-string formatting in conformance with the `flags` package.
func (i *List) String() string {
	var els []string
	for _, c := range *i {
		els = append(els, c.String())
	}
	return strings.Join(els, ", ")
}

// Set provides string-parsing in conformance with the `flags` package.
// Calling Set parses 'value' and adds it as Consumer to the list 'i'.
func (i *List) Set(value string) error {
	c, err := Parse(value)
	if err != nil {
		return err
	}

	*i = append(*i, c)
	return nil
}

// LastChange returns the most recent change in the list of consumer
// or zero-time if none have changed ever.
func (i *List) LastChange() time.Time {
	var max time.Time
	for _, g := range *i {
		if lastChange := g.LastChange(); !lastChange.IsZero() && (max.IsZero() || lastChange.After(max)) {
			max = lastChange
		}
	}
	return max
}

func (i *List) Close() {
	for _, g := range *i {
		g.Close()
	}
}
