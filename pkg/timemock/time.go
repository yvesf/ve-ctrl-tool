// timemock is a thin wrapper over stdlib/time package that overloads a few
// functions to allow time manipulation in tests.
package timemock

import "time"

var (
	Now        = time.Now
	After      = time.After
	Sleep      = time.Sleep
	NewTimer   = func(d time.Duration) timer { return timer{time.NewTimer(d), 1} }
	wakeupChan = make(chan struct{})
)

type timer struct {
	*time.Timer
	factor int
}

func (t *timer) Reset(d time.Duration) bool {
	return t.Timer.Reset(d / time.Duration(t.factor))
}

func TimeWarp(factor int) func() {
	start := time.Now()
	Now = func() time.Time {
		realNow := time.Now()
		now := realNow.Add(realNow.Sub(start) * time.Duration(factor))
		return now
	}
	After = func(d time.Duration) <-chan time.Time {
		return time.After(d / time.Duration(factor))
	}
	Sleep = func(d time.Duration) {
		time.Sleep(d / time.Duration(factor))
	}
	NewTimer = func(d time.Duration) timer {
		return timer{time.NewTimer(d / time.Duration(factor)), factor}
	}
	return func() {
		Now = time.Now
		After = time.After
		Sleep = time.Sleep
		NewTimer = func(d time.Duration) timer { return timer{time.NewTimer(d), 1} }
	}
}

func Freeze(at time.Time) func() {
	Now = func() time.Time {
		return at
	}
	After = func(d time.Duration) <-chan time.Time {
		ch := make(chan time.Time, 1)
		end := Now().Add(d)
		go func() {
			for range wakeupChan {
				if n := Now(); n.After(end) {
					ch <- n
					return
				}
			}
		}()
		return ch
	}
	Sleep = func(d time.Duration) {
		end := Now().Add(d)
		for Now().Before(end) {
			<-wakeupChan
		}
	}
	NewTimer = func(d time.Duration) timer {
		panic("not supported")
	}

	wakeup()
	return func() {
		Now = time.Now
		After = time.After
		Sleep = time.Sleep
		NewTimer = func(d time.Duration) timer { return timer{time.NewTimer(d), 1} }
		wakeup()
	}
}

func wakeup() {
	for {
		select {
		case wakeupChan <- struct{}{}:
		default:
			return
		}
	}
}
