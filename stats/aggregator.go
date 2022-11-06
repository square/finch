// Copyright 2022 Block, Inc.

package stats

import (
	"log"

	"github.com/square/finch"
	"github.com/square/finch/config"
)

// Aggregator aggregates stats from local and remote collectors.
type Ag struct {
	reporters  []Reporter
	agChan     chan Stats
	nInstances uint
	stopChan   chan struct{}
	doneChan   chan struct{}
}

func NewAg(nInstances uint, cfg config.Stats) (*Ag, error) {
	reporters, err := MakeReporters(cfg)
	if err != nil {
		return nil, err
	}
	return &Ag{
		reporters:  reporters,
		agChan:     make(chan Stats, nInstances),
		nInstances: nInstances,
		stopChan:   make(chan struct{}),
		doneChan:   make(chan struct{}),
	}, nil
}

func (ag *Ag) Chan() chan<- Stats {
	return ag.agChan
}

func (ag *Ag) Done() {
	finch.Debug("ag stop")
	close(ag.stopChan)            // stop Run
	<-ag.doneChan                 // wait for last stats to be reported
	for i := range ag.reporters { // wait for reporters
		ag.reporters[i].Stop()
	}
}

func (ag *Ag) Run() {
	defer close(ag.doneChan)

	var s Stats
	i := uint(0)                             // interval[i]
	interval := make([]Stats, ag.nInstances) // stats per compute
	intervalNo := uint(1)
	for {
		select {
		case s = <-ag.agChan:
			finch.Debug("recv stats (interval %d, n %d): %+v", intervalNo, i, s)
		case <-ag.stopChan:
			return
		}

		if s.Interval < intervalNo {
			// Some compute sent past interval (network delay?); discard it
			// so we don't have to rewrite history.
			log.Printf("Discarding past stats: %+v", s)
			continue
		}

		if s.Interval > intervalNo {
			// Some compute sent next interval before current one is complete.
			// Report whatever stats we have for current interval.
			log.Printf("Received next stats interval (%d) before current interval (%d) complete; reporting incomplete current interval; next stats: %+v", s.Interval, intervalNo, s)
			ag.report(interval)
			interval[0] = s
			i = 1
			intervalNo += 1
			continue
		}

		// Stats in current interval; buffer until we've received all stats
		interval[i] = s
		i++
		if i < ag.nInstances {
			continue // wait for more stats in this interval
		}

		// Received all stats in this interval
		finch.Debug("interval %d complete", intervalNo)
		ag.report(interval)
		i = 0
		intervalNo += 1
	}
}

func (ag *Ag) report(interval []Stats) {
	for i := range ag.reporters {
		ag.reporters[i].Report(interval)
	}
}
