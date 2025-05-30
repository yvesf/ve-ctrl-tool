package ringbuf_test

import (
	"math"
	"testing"

	"github.com/carlmjohnson/be"

	"github.com/yvesf/ve-ctrl-tool/pkg/ringbuf"
)

func inRange(t *testing.T, expected, got, delta float64) {
	t.Helper()
	if got < expected-delta {
		t.Fatalf("%f < %f-%f", got, expected, delta)
	}
	if got > expected+delta {
		t.Fatalf("%f > %f+%f", got, expected, delta)
	}
}

func TestRingbuf(t *testing.T) {
	t.Run(`empty`, func(t *testing.T) {
		r := ringbuf.NewRingbuf(0)
		be.True(t, math.IsNaN(r.Mean()))
		r.Add(1)
		be.True(t, math.IsNaN(r.Mean()))
	})
	t.Run(`size -1 invalid`, func(t *testing.T) {
		r := ringbuf.NewRingbuf(-1)
		be.True(t, math.IsNaN(r.Mean()))
		r.Add(1)
		be.True(t, math.IsNaN(r.Mean()))
	})
	t.Run(`size 1`, func(t *testing.T) {
		r := ringbuf.NewRingbuf(1)
		be.True(t, math.IsNaN(r.Mean()))
		r.Add(2)
		be.Equal(t, float64(2), r.Mean())
		r.Add(100)
		be.Equal(t, float64(100), r.Mean())
	})
	t.Run(`size 5`, func(t *testing.T) {
		r := ringbuf.NewRingbuf(5)
		be.True(t, math.IsNaN(r.Mean()))
		r.Add(1)
		be.Equal(t, float64(1), r.Mean())
		r.Add(2)
		inRange(t, float64(1.5), r.Mean(), 0.0001)
		r.Add(1)
		inRange(t, float64(1.3), r.Mean(), 0.04)
		r.Add(1)
		inRange(t, float64(1.25), r.Mean(), 0.0001)
		r.Add(1)
		inRange(t, float64(1.2), r.Mean(), 0.00001)
		r.Add(1)
		inRange(t, float64(1.2), r.Mean(), 0.00001)
		r.Add(1)
		inRange(t, float64(1), r.Mean(), 0.0001)
		r.Add(-100)
		inRange(t, float64(-19.2), r.Mean(), 0.0001)
	})
}
