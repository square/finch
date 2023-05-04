// Copyright 2023 Block, Inc.

package stats

import (
	"fmt"
	"log"
	"time"

	"github.com/square/finch"
	"github.com/square/finch/config"
)

var Now func() time.Time = time.Now

// Instance stats are per trx and total (all trx) stats from all clients on a
// local or report instance. N-many instances constitute an interval of N instance
// stats. Collector.Recv waits for stats to complete each interval before reporting.
type Instance struct {
	Hostname string            // local or remote compute
	Clients  uint              // number of clients
	Interval uint              // interval number, monotonically incr
	Seconds  float64           // of interval
	Runtime  uint              // total elapsed seconds of benchmark
	Total    *Stats            // all trx stats combined
	Trx      map[string]*Stats // per trx stats
}

// Combine combines instance stats for the same interval.
func (in *Instance) Combine(from []Instance) {
	in.Hostname = fmt.Sprintf("(%d combined)", len(from))
	in.Clients = from[0].Clients
	in.Interval = from[0].Interval
	in.Seconds = from[0].Seconds
	in.Runtime = from[0].Runtime
	in.Total.Copy(from[0].Total) // copy the first
	for i := range from[1:] {    // combine the rest
		in.Total.Combine(from[1+i].Total)
		in.Clients += from[1+i].Clients
	}
}

// Collector collects and reports stats from local and remote instances.
// If config.stats.freq is set, stats are collected/reported at that frequency.
// Else, they're collected/reported once when the stage finishes.
type Collector struct {
	Freq       time.Duration
	trx        [][]*Trx   // lock-free trx stats per client
	stats      [][]*Stats // stats per trx (per client)
	local      Instance   // local instance stats
	interval   []Instance // all Instance stats
	n          uint       // index in interval
	nInstances uint       // number of instances in interval
	intervalNo uint       // current interval being filled
	stopChan   chan struct{}
	doneChan   chan struct{}
	start      time.Time // when Start was called
	last       time.Time // when Collect was last called
	reporters  []Reporter
	stopped    bool
	finalChan  chan struct{}
}

func NewCollector(cfg config.Stats, hostname string, nInstances uint) (*Collector, error) {
	finch.Debug("stats: %+v %s %d", cfg, hostname, nInstances)
	freq, _ := time.ParseDuration(cfg.Freq) // already validated

	reporters, err := MakeReporters(cfg)
	if err != nil {
		return nil, err
	}

	return &Collector{
		Freq:     freq,
		stopChan: make(chan struct{}),
		doneChan: make(chan struct{}),
		local: Instance{
			Hostname: hostname,
			Total:    NewStats(),
			Trx:      map[string]*Stats{},
		},
		interval:   make([]Instance, nInstances),
		nInstances: nInstances,
		reporters:  reporters,
		intervalNo: 1,
		finalChan:  make(chan struct{}),
	}, nil
}

func (c *Collector) Done() chan struct{} {
	return c.finalChan
}

// Watch all trx stats from one client. This must be called for each Client
// because it determines what Collect collects.
func (c *Collector) Watch(trx []*Trx) {
	c.local.Clients += 1
	c.trx = append(c.trx, make([]*Trx, len(trx)))
	c.stats = append(c.stats, make([]*Stats, len(trx)))
	n := len(c.trx) - 1
	for i := range trx {
		c.trx[n][i] = trx[i]
		c.stats[n][i] = nil // fetch value later in report
		if _, ok := c.local.Trx[trx[i].Name]; !ok {
			c.local.Trx[trx[i].Name] = NewStats()
		}
	}
}

// Start starts metrics collection. It's called only once immediately before
// starting clients in Stage.Run. If periodic stats are enabled (config.stats.freq > 0),
// a goroutine is started to call Collect at the configured frequency, which is
// stopped when Stop is called.
func (c *Collector) Start() {
	finch.Debug("start (freq %s)", c.Freq)
	now := Now()
	c.start = now
	c.last = now
	if c.Freq == 0 {
		return
	}

	// Collect stats periodically; stopped by Stop
	go func() {
		ticker := time.NewTicker(c.Freq)
		for { // ticker
			select {
			case <-ticker.C:
				c.Collect()
			case <-c.stopChan:
				close(c.doneChan)
				ticker.Stop()
				return
			}
		}
	}()
}

// Stop stops metrics collect. It's called only once immediately after the stage
// finishes (in Stage.Run). It stops the goroutine started in Start, if periodic
// stats are enabled (config.stats.freq > 0).
func (c *Collector) Stop() {
	c.stopped = true
	finch.Debug("stop")
	if c.Freq > 0 {
		// Handle reporting race condition. Suppose freq=2s and runtime=2s.
		// When run ends, either the ticker in that ^ goroutine can exec first
		// and report the last interval, or closing stopChan can exec first
		// and close the goroutine, causing last interval not to be reported.
		// So take intervalNo before and after stopping goroutine. If it
		// incremented, then we know report() had run; if not, we know that
		// report() did not run, so we fall through to do final Collect().
		n0 := c.intervalNo
		close(c.stopChan) // stop goroutine ^
		<-c.doneChan      // wait for final report
		n1 := c.intervalNo
		if n1 > n0 {
			finch.Debug("final interval reported")
			return
		}
	}
	finch.Debug("reporting final interval")
	c.Collect()
}

// Collect collects stats from all clients. It's called periodically by the
// goroutine in Start, or once by Stop if periodic stats aren't enabled.
func (c *Collector) Collect() bool {
	finch.Debug("collect")

	// End of this interval
	now := Now()
	c.local.Interval += 1
	c.local.Seconds = now.Sub(c.last).Seconds()
	c.last = now

	// Update total runtime: calculated from c.start, not c.last, and reported
	// as whole seconds because it's used as X axis of graphs
	c.local.Runtime = uint(now.Sub(c.start).Seconds())

	// Lock-free swap: each Trx does an atomic pointer swap of its internal
	// "a" and "b" stats. So if *a.Stats is active now, Swap swaps to *b.Stats
	// and returns *a.Stats. The pointer is owned by the Trx so DO NOT modify it;
	// we just borrow the pointer to report the stats.
	for i := range c.trx {
		for j := range c.trx[i] {
			c.stats[i][j] = c.trx[i][j].Swap()
		}
	}

	// Combine all trx stats into total stats
	c.local.Total.Reset()
	seen := map[string]bool{}
	for i := range c.trx {
		for j := range c.trx[i] {
			trxName := c.trx[i][j].Name
			s := c.stats[i][j] // *Stats: DO NOT modify; see comment ^

			// Reset our local copy first time we see the trx each interval
			if !seen[trxName] {
				c.local.Trx[trxName].Reset()
				seen[trxName] = true
			}

			// Merge stats into our local copies
			c.local.Trx[trxName].Combine(s)
			c.local.Total.Combine(s)
		}
	}

	// "Send" local instance stats to ourself. Think of it like sending the stats
	// to 127.0.0.1. This abstraction is needed for remote instances reporting
	// their stats to this instance, which happens when compute/Server.remoteStats
	// calls Recv, passing remote stats.
	return c.Recv(c.local)
}

// Recv receives stats from one local or remote client. If local, it's called by
// Collect. If remote, it's called by compute/Server.remoteStats after receiving
// stats from the remote instance. Stats are reported when the interval is complete
// (when N stats from N instances have been received); until then, they're buffered.
func (c *Collector) Recv(in Instance) bool {
	finch.Debug("recv %+v", in)

	// Is the received interval in the past? This can happen for stats from remote
	// instances if, for example, there's a really bad network delay. Since the old
	// interval has already been reported, and we don't buffer or report intervals
	// out of order, we just have to drop the old/delayed interval.
	if in.Interval < c.intervalNo {
		log.Printf("Discarding past stats: %+v", in)
		return false
	}

	// Reverse of above: is received interval in the future? If yes, then report
	// the current interval because it must not have filled up (else it would have
	// reported earlier). This can happen if stats from one or more remote instance
	// are lost, so the interval doesn't complete. This will report a partial interval.
	if in.Interval > c.intervalNo {
		log.Printf("Received next stats interval (%d) before current interval (%d) complete; reporting incomplete current interval; next stats: %+v", in.Interval, c.intervalNo, in)
		c.Report()
		c.interval[0] = in
		c.n = 1
		return true
	}

	// Stats in current interval; buffer until we've received all stats
	c.interval[c.n] = in
	c.n += 1
	if c.n < c.nInstances {
		finch.Debug("have %d of %d", c.n, c.nInstances)
		return false // wait for more stats in this interval
	}

	// Received all stats in this interval
	finch.Debug("interval %d complete", c.intervalNo)
	c.Report()
	return true
}

func (c *Collector) Report() {
	for _, r := range c.reporters {
		r.Report(c.interval[0:c.n])
	}
	c.intervalNo += 1
	c.n = 0
	if c.stopped {
		finch.Debug("stopping reporters")
		for _, r := range c.reporters {
			r.Stop()
		}
		close(c.finalChan)
	}
}
