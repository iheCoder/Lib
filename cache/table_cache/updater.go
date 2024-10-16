package table_cache

import (
	"github.com/RussellLuo/timingwheel"
	"time"
)

type tableOpScheduler struct {
	baseInterval time.Duration
}

func (s *tableOpScheduler) Next(prev time.Time) time.Time {
	return prev.Add(s.baseInterval)
}

func (mgr *TableCacheMgr) startUpdateOpsData() {
	// start a timing wheel
	tw := timingwheel.NewTimingWheel(time.Second, 60)
	tw.Start()
	defer tw.Stop()

	// add update ops to timing wheel
	for key, op := range mgr.ops {
		tw.ScheduleFunc(&tableOpScheduler{op.config.UpdateInterval}, func() {
			mgr.pullTableData(*op.config, key)
		})
	}

	// wait for kill signal
	<-mgr.cancelSignal
}
