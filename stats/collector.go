// Copyright 2023 Block, Inc.

package stats

import (
	"fmt"
	"log"
	"sync"
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
	Runtime  float64           // total elapsed seconds of benchmark
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
// Else, they're collected/reported once when the stage finishes and calls Stop.
type Collector struct {
	Freq       time.Duration
	trx        [][]*Trx   // lock-free trx stats per client
	stats      [][]*Stats // stats per trx (per client)
	local      Instance   // local instance stats
	nInstances uint       // number of instances in interval
	stopChan   chan struct{}
	doneChan   chan struct{}
	start      time.Time // when Start was called, calculates Runtime
	last       time.Time // when Collect was last called
	reporters  []Reporter
	finalChan  chan struct{}

	*sync.Mutex
	intervalNo uint       // current interval being filled
	interval   []Instance // all Instance stats
	n          uint       // index in interval
	reported   time.Time  // when Report was last called
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
		Mutex:      &sync.Mutex{},
	}, nil
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
				finch.Debug("stop ticker")
				close(c.doneChan)
				ticker.Stop()
				return
			}
		}
	}()
}

// Stop stops metrics collection, waits for final stats, and prints the final report.
// It's called once immediately after the stage finishes (in Stage.Run). It stops the
// goroutine started in Start, if periodic stats are enabled (stats.freq > 0).
func (c *Collector) Stop(timeout time.Duration, terminated bool) bool {
	/*
		This func is necessarily complex because there's a race condition that
		can't solved with basic synchronization because the problem is related
		to the ticker in Start. For example, suppose runtime=10s and stats.freq=5s.
		At 10s, stage.Run is interrupted, waits for clients to stop, and then
		calls this Stop. Meanwhile, the Start goroutine and ticker ^ are running
		independently. The problem is: does Stop (here) happen before or after
		the 2nd tick at 10s ("tick->Collect[2]" in the cases below)? If it happens
		_before_ Stop stops the ticker with close(stopChan), then good (cases A and C):
		the final tick was received, which triggered the final report at 10s.
		But it can also happen that Stop stops the ticker right before the final
		tick is received (case B), so the final report will be lost unless Stop
		invokes it. [N] is the goroutine number, call order top to bottom:

		  Case A				Case B				Case C
		  ------				------				------
		  Stop[1]				Stop[1]				tick->Collect[2]
		  tick->Collect[2]		close(stopChan)[1]	Stop[1]
		  close(stopChan)[1]						close(stopChan)[1]

		  Here's --debug for case C:

			2023/06/30 16:16:11.807782 DEBUG stage.go:183 stage runtime elapsed
			2023/06/30 16:16:11.807834 DEBUG stage.go:198 spin wait for 1 clients
			2023/06/30 16:16:11.807862 DEBUG collector.go:260 collect
			2023/06/30 16:16:11.807864 DEBUG stage.go:203 1(read-only)/e1(dml1)/g1/c1/t0()/q0 done: <nil>
			2023/06/30 16:16:11.807919 DEBUG collector.go:353 interval 2: complete
				<...stats reported...>
			2023/06/30 16:16:11.808993 DEBUG collector.go:131 stop ticker
			2023/06/30 16:16:11.809021 DEBUG collector.go:214 last report: 29.641µs ago
			2023/06/30 16:16:11.809038 DEBUG collector.go:217 final report done

		The stage stops, then "collector.go:260 collect" is the call from Start/final tick,
		then "collector.go:131 stop ticker" happens due to "close(stopChan)[1]" in Stop.
		Now here's --debug for case B:

			2023/06/30 16:14:05.495255 DEBUG stage.go:183 stage runtime elapsed
			2023/06/30 16:14:05.495291 DEBUG stage.go:198 spin wait for 1 clients
			2023/06/30 16:14:05.495330 DEBUG stage.go:203 1(read-only)/e1(dml1)/g1/c1/t0()/q0 done: <nil>
			2023/06/30 16:14:05.495352 DEBUG collector.go:131 stop ticker
			2023/06/30 16:14:05.495373 DEBUG collector.go:214 last report: 1.998968824s ago
			2023/06/30 16:14:05.495384 DEBUG collector.go:220 last periodic collect
			2023/06/30 16:14:05.495392 DEBUG collector.go:260 collect
			2023/06/30 16:14:05.495411 DEBUG collector.go:353 interval 2: complete

		Notice "stop ticker" before "collector.go:260 collect": Go executed "close(stopChan)[1]"
		before the final tick was received, and "last report: 1.998968824s ago" proves it
		(stats.freq=2s): the final tick was 0.001031176s away (or late).

		This problem exists not only because of random goroutine run ordering,
		but also because we can't "ask" the ticker if the final tick was received
		for three reasons: 1) time.Ticker has no "final tick", it just ticks forever,
		and 2) we can't compute a final tick like numberOfTicks = runtime / freq
		because runtime might not be set (an iter limitation or CTRL-C can stop the run).
		The 3rd reason is even tricker...

		So far we've been talking about the local compute instance, so all this happens
		within a matter of nanoseconds. But with _remote_ compute instances,
		the final report can be very delayed due to network delays. Before the last tick,
		we just wait between ticks and report late. But on the last tick, we need to
		shutdown as quickly as possible but also wait for the last metrics from the
		remotes. So even after handling cases A-C, we have to loop without a timeout
		until Report returns true, which means all stats received and reported.
	*/

	reported := false
	var lastReported time.Duration
	if c.Freq == 0 {
		reported = c.Collect() // first/last/only collection
	} else {
		close(c.stopChan) // stop goroutine in Start ^
		<-c.doneChan      // wait for Start to return
		if terminated {
			//finch.Debug("terminated; not waiting for next report")
			//reported = true
			//goto STOP
		}
	}

	c.Lock()
	lastReported = time.Now().Sub(c.reported)
	c.Unlock()
	finch.Debug("last report: %s ago", lastReported)

	if reported || lastReported < (c.Freq/2) {
		finch.Debug("final report done")
		reported = true
	} else {
		if c.Freq > 0 {
			finch.Debug("last periodic collect")
			reported = c.Collect()
			if reported {
				goto STOP
			}
		}

		finch.Debug("waiting %s for final report...", timeout)
		timeoutC := time.After(timeout)
	WAIT:
		for !reported {
			select {
			case <-timeoutC:
				c.Lock()
				reported = c.Report(true) // true=force
				c.Unlock()
				break WAIT
			default:
				time.Sleep(100 * time.Millisecond)
			}
			c.Lock()
			reported = c.Report(false)
			c.Unlock()
		}
	}

STOP:
	finch.Debug("stopping reporters")
	for _, r := range c.reporters {
		r.Stop()
	}

	finch.Debug("collector stopped")
	close(c.finalChan)
	return reported
}

// Collect collects stats from all local clients. It's called periodically by
// the goroutine in Start, or once by Stop if periodic stats aren't enabled.
func (c *Collector) Collect() bool {
	finch.Debug("collect")

	// End of this interval
	now := Now()
	c.local.Interval += 1
	c.local.Seconds = now.Sub(c.last).Seconds()
	c.last = now

	// Update total runtime: calculated from c.start, not c.last
	c.local.Runtime = now.Sub(c.start).Seconds()

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

	c.Lock()
	defer c.Unlock()
	c.interval[c.n] = c.local
	c.n++
	return c.Report(false)
}

// Recv receives stats from remote compute instances. It's called by
// compute/Server.remoteStats.
func (c *Collector) Recv(in Instance) {
	finch.Debug("recv %+v", in)
	c.Lock()
	defer c.Unlock()

	// Is the received interval in the past? This can happen for stats from remote
	// instances if, for example, there's a really bad network delay. Since the old
	// interval has already been reported, and we don't buffer or report intervals
	// out of order, we just have to drop the old/delayed interval.
	if in.Interval < c.intervalNo {
		log.Printf("Discarding past stats: %+v", in)
		return
	}

	// Reverse of above: is received interval in the future? If yes, then report
	// the current interval because it must not have filled up (else it would have
	// reported earlier). This can happen if stats from one or more remote instance
	// are return  lost, so the interval doesn't complete. This will report a partial interval.
	if in.Interval > c.intervalNo {
		log.Printf("Received next stats interval (%d) before current interval (%d) complete; reporting incomplete current interval; next stats: %+v", in.Interval, c.intervalNo, in)
		c.Report(true) // true=force
		c.interval[0] = in
		c.n = 1
		return
	}

	// Stats in current interval; buffer until we've received all stats
	c.interval[c.n] = in
	c.n += 1
	c.Report(false)
}

// Report reports stats when then current interval is completed: when there are
// stats from all instances (local and remote). Until the interval is complete,
// Report does nothing and returns false, unless force is true to force reporting
// an incomplete interval, which happens when Stop(timeout) times out.
func (c *Collector) Report(force bool) bool {
	if force {
		finch.Debug("interval %d: forcing with %d of %d instances", c.intervalNo, c.n, c.nInstances)
	} else if c.n < c.nInstances {
		finch.Debug("interval %d: not complete: have %d of %d", c.intervalNo, c.n, c.nInstances)
		return false // wait for more stats in this interval
	} else {
		finch.Debug("interval %d: complete", c.intervalNo)
	}
	for _, r := range c.reporters {
		r.Report(c.interval[0:c.n])
	}
	c.reported = time.Now()
	c.intervalNo += 1
	c.n = 0
	return true // interval complete and reported
}
