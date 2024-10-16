package table_cache

import (
	"github.com/RussellLuo/timingwheel"
	"math/rand/v2"
	"time"
)

const (
	defaultWheelInterval = time.Second
)

type tableOpScheduler struct {
	baseInterval time.Duration
}

func (s *tableOpScheduler) Next(prev time.Time) time.Time {
	return prev.Add(s.baseInterval)
}

func (mgr *TableCacheMgr) startUpdateOpsData() {
	// start a timing wheel
	tw := timingwheel.NewTimingWheel(defaultWheelInterval, 60)
	tw.Start()
	defer tw.Stop()

	// add update ops to timing wheel
	for key, op := range mgr.ops {
		// if no update interval, skip
		if op.config.UpdateInterval <= 0 {
			continue
		}

		// add random interval to avoid thundering herd
		interval := getRandomInterval(op.config.UpdateInterval)
		tw.ScheduleFunc(&tableOpScheduler{interval}, func() {
			mgr.pullTableData(*op.config, key)
		})
	}

	// wait for kill signal
	<-mgr.cancelSignal
}

func getRandomInterval(base time.Duration) time.Duration {
	elapse := time.Duration(rand.Int64N(int64(defaultWheelInterval*2))) - defaultWheelInterval
	return base + elapse
}
