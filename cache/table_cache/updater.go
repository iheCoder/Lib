package table_cache

import (
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
	mgr.tw.Start()
	defer mgr.tw.Stop()

	// wait for kill signal
	<-mgr.cancelSignal
}

func (mgr *TableCacheMgr) addToUpdateWheel(op *TableCacheOp) {
	// if no update interval, skip
	if op.config.UpdateInterval <= 0 {
		return
	}

	// add random interval to avoid thundering herd
	interval := getRandomInterval(op.config.UpdateInterval)
	mgr.tw.ScheduleFunc(&tableOpScheduler{interval}, func() {
		mgr.updateOpData(op)
	})
}

func getRandomInterval(base time.Duration) time.Duration {
	elapse := time.Duration(rand.Int64N(int64(defaultWheelInterval*2))) - defaultWheelInterval
	return base + elapse
}
