package correct

import (
	"fmt"
	"time"
)

type ProgressReporter struct {
	startTime, endTime time.Time
	totalDuration      time.Duration
	ks                 map[string]*keyStage
}

func NewProgressReporter() *ProgressReporter {
	return &ProgressReporter{
		ks: make(map[string]*keyStage),
	}
}

func (p *ProgressReporter) StartRecord() {
	p.startTime = time.Now()
}

func (p *ProgressReporter) EndRecord() {
	p.endTime = time.Now()
	p.totalDuration = p.endTime.Sub(p.startTime)
}

func (p *ProgressReporter) StartKeyStageRecord(name string) {
	if _, ok := p.ks[name]; !ok {
		p.ks[name] = &keyStage{
			name:        name,
			minDuration: time.Duration(1<<63 - 1),
		}
	}

	p.ks[name].startTime = time.Now()
}

func (p *ProgressReporter) EndKeyStageRecord(name string) {
	p.ks[name].endTime = time.Now()
	currentDuration := p.ks[name].endTime.Sub(p.ks[name].startTime)
	p.ks[name].totalDuration += currentDuration
	p.ks[name].count++

	if currentDuration > p.ks[name].maxDuration {
		p.ks[name].maxDuration = currentDuration
	}
	if currentDuration < p.ks[name].minDuration {
		p.ks[name].minDuration = currentDuration
	}
}

func (p *ProgressReporter) Report() {
	// print total duration
	fmt.Printf("Total duration: %d s\n", p.totalDuration/time.Second)

	// print key stage
	for k, v := range p.ks {
		fmt.Printf("\t Key: %s, Count: %d, Total duration: %d s, Max duration: %d s, Min duration: %d s\n", k, v.count, v.totalDuration/time.Second, v.maxDuration/time.Second, v.minDuration/time.Second)
	}
}

type keyStage struct {
	name                     string
	totalDuration            time.Duration
	count                    int
	maxDuration, minDuration time.Duration
	startTime, endTime       time.Time
}
