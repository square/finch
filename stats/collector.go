// Copyright 2022 Block, Inc.

package stats

import (
	"time"

	"github.com/square/finch"
	"github.com/square/finch/config"
)

// Collector collects stats from all local clients at config.stats.freq intervals.
// Stats are combined and sent to the aggregator, which sends them to reporters.
type Collector struct {
	freq     time.Duration
	all      []*Stats
	stopChan chan struct{}
	agChan   chan<- Stats
	interval uint
	name     string
	start    time.Time
}

func NewCollector(cfg config.Stats, name string, agChan chan<- Stats) *Collector {
	finch.Debug("stats: %+v", cfg)
	freq, _ := time.ParseDuration(cfg.Freq) // already validated
	return &Collector{
		freq:     freq,
		stopChan: make(chan struct{}),
		agChan:   agChan,
		name:     name,
	}
}

func (r *Collector) Watch(s *Stats) {
	r.all = append(r.all, s)
}

func (r *Collector) Start() {
	finch.Debug("stats.Collector start")
	r.start = time.Now()
	if r.freq > 0 {
		go r.collect()
	}
}

func (r *Collector) Stop() {
	finch.Debug("stats.Collector stop")
	if r.freq > 0 {
		close(r.stopChan)
	}
	if r.freq == 0 {
		r.interval++
		r.report() // entire runtime
	}
}

func (r *Collector) collect() {
	ticker := time.NewTicker(r.freq)
	defer ticker.Stop()
	for { // ticker
		select {
		case <-ticker.C:
			r.interval++
			r.report()
		case <-r.stopChan:
			return
		}
	}
}

func (r *Collector) report() {
	runtime := time.Now().Sub(r.start).Round(time.Second)
	total := NewStats() // all clients
	total.Interval = r.interval
	total.Compute = r.name
	total.Clients = len(r.all)
	total.Runtime = uint(runtime.Seconds())
	for _, s := range r.all {
		total.Combine(s.Snapshot())
	}
	r.agChan <- *total // ag debug-prints the stats on recv
}
