package table_cache

import (
	"github.com/RussellLuo/timingwheel"
	"testing"
	"time"
)

const (
	acceptanceInterval = 1 * time.Second
)

type twDataChecker struct {
	prev             time.Time
	expectedInterval time.Duration
}

func (t *twDataChecker) checkIfFine() bool {
	defer func() {
		t.prev = time.Now()
	}()

	realInterval := time.Since(t.prev)
	return realInterval >= t.expectedInterval-acceptanceInterval && realInterval <= t.expectedInterval+acceptanceInterval
}

func TestTimingWheelTableOpScheduler(t *testing.T) {
	// start a timing wheel
	tw := timingwheel.NewTimingWheel(time.Second, 60)
	tw.Start()
	defer tw.Stop()

	// add one second, two seconds scheduler to timing wheel
	scheduler1 := &tableOpScheduler{time.Second}
	scheduler2 := &tableOpScheduler{2 * time.Second}

	checker1 := &twDataChecker{prev: time.Now(), expectedInterval: time.Second}
	checker2 := &twDataChecker{prev: time.Now(), expectedInterval: 2 * time.Second}

	// check if the interval is as expected
	tw.ScheduleFunc(scheduler1, func() {
		if !checker1.checkIfFine() {
			t.Errorf("scheduler1 interval not as expected")
		}
	})
	tw.ScheduleFunc(scheduler2, func() {
		if !checker2.checkIfFine() {
			t.Errorf("scheduler2 interval not as expected")
		}
	})

	// wait for the timing wheel to finish
	time.Sleep(10 * time.Second)
}
