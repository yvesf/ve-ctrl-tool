package ringbuf

// Ringbuf is a simple circular buffer to calculate the average value of a series.
type Ringbuf struct {
	buf []float64
	p   int
	s   int
}

func NewRingbuf(size int) *Ringbuf {
	return &Ringbuf{
		s: size,
	}
}

func (r *Ringbuf) Add(v float64) {
	if r.s <= 0 {
		return
	}
	if len(r.buf) < r.s {
		r.buf = append(r.buf, v)
		return
	}
	r.buf[r.p] = v
	r.p = (r.p + 1) % r.s
}

func (r *Ringbuf) Mean() float64 {
	var sum float64
	for _, v := range r.buf {
		sum += v
	}
	return sum / float64(len(r.buf))
}
