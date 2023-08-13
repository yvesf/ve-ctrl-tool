package ringbuf_test

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yvesf/ve-ctrl-tool/pkg/ringbuf"
)

func TestRingbuf(t *testing.T) {
	t.Run(`empty`, func(t *testing.T) {
		r := ringbuf.NewRingbuf(0)
		require.True(t, math.IsNaN(r.Mean()))
		r.Add(1)
		require.True(t, math.IsNaN(r.Mean()))
	})
	t.Run(`size -1 invalid`, func(t *testing.T) {
		r := ringbuf.NewRingbuf(-1)
		require.True(t, math.IsNaN(r.Mean()))
		r.Add(1)
		require.True(t, math.IsNaN(r.Mean()))
	})
	t.Run(`size 1`, func(t *testing.T) {
		r := ringbuf.NewRingbuf(1)
		require.True(t, math.IsNaN(r.Mean()))
		r.Add(2)
		require.Equal(t, float64(2), r.Mean())
		r.Add(100)
		require.Equal(t, float64(100), r.Mean())
	})
	t.Run(`size 5`, func(t *testing.T) {
		r := ringbuf.NewRingbuf(5)
		require.True(t, math.IsNaN(r.Mean()))
		r.Add(1)
		require.Equal(t, float64(1), r.Mean())
		r.Add(2)
		require.InEpsilon(t, float64(1.5), r.Mean(), 0.0001, r.Mean())
		r.Add(1)
		require.InEpsilon(t, float64(1.3), r.Mean(), 0.03, r.Mean())
		r.Add(1)
		require.InEpsilon(t, float64(1.25), r.Mean(), 0.0001, r.Mean())
		r.Add(1)
		require.InEpsilon(t, float64(1.2), r.Mean(), 0.00001, r.Mean())
		r.Add(1)
		require.InEpsilon(t, float64(1.2), r.Mean(), 0.00001, r.Mean())
		r.Add(1)
		require.InEpsilon(t, float64(1), r.Mean(), 0.0001, r.Mean())
		r.Add(-100)
		require.InEpsilon(t, float64(-19.2), r.Mean(), 0.0001, r.Mean())
	})
}
