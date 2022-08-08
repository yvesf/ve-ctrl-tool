// Package backoff is extracted from https://github.com/olivere/elastic
// Copyright 2012-present Oliver Eilhard. MIT License.
package backoff

import (
	"math"
	"math/rand"
	"time"
)

// Backoff allows callers to implement their own Backoff strategy.
type Backoff interface {
	// Next implements a BackoffFunc.
	Next(retry int) (time.Duration, bool)
}

// -- Exponential --

// ExponentialBackoff implements the simple exponential backoff described by
// Douglas Thain at http://dthain.blogspot.de/2009/02/exponential-backoff-in-distributed.html.
type ExponentialBackoff struct {
	t float64 // initial timeout (in msec)
	f float64 // exponential factor (e.g. 2)
	m float64 // maximum timeout (in msec)
}

// NewExponentialBackoff returns a ExponentialBackoff backoff policy.
// Use initialTimeout to set the first/minimal interval
// and maxTimeout to set the maximum wait interval.
func NewExponentialBackoff(initialTimeout, maxTimeout time.Duration) *ExponentialBackoff {
	return &ExponentialBackoff{
		t: float64(int64(initialTimeout / time.Millisecond)),
		f: 2.0,
		m: float64(int64(maxTimeout / time.Millisecond)),
	}
}

// Next implements BackoffFunc for ExponentialBackoff.
func (b *ExponentialBackoff) Next(retry int) (time.Duration, bool) {
	r := 1.0 + rand.Float64() // random number in [1..2]
	m := math.Min(r*b.t*math.Pow(b.f, float64(retry)), b.m)
	if m >= b.m {
		return 0, false
	}
	d := time.Duration(int64(m)) * time.Millisecond
	return d, true
}
