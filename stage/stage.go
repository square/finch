// Copyright 2024 Block, Inc.

package stage

import (
	"context"
	"log"
	"runtime/pprof"
	"time"

	"github.com/square/finch"
	"github.com/square/finch/client"
	"github.com/square/finch/config"
	"github.com/square/finch/data"
	"github.com/square/finch/dbconn"
	"github.com/square/finch/limit"
	"github.com/square/finch/stats"
	"github.com/square/finch/trx"
	"github.com/square/finch/workload"
)

// Stage allocates and runs a workload. It handles stats for the workload,
// including reporting. A stage has a two-phase execute: Prepare to set up
// everything, then Run to execute clients (which execute queries). Run is
// optional; it's not run when --test is specified on the command line.
type Stage struct {
	cfg   config.Stage
	gds   *data.Scope
	stats *stats.Collector
	// --
	doneChan   chan *client.Client      // <-Client.Run()
	execGroups [][]workload.ClientGroup // [n][Client]
}

func New(cfg config.Stage, gds *data.Scope, stats *stats.Collector) *Stage {
	return &Stage{
		cfg:   cfg,
		gds:   gds,
		stats: stats,
		// --
		doneChan: make(chan *client.Client, 1),
	}
}

func (s *Stage) Prepare(ctxFinch context.Context) error {
	if len(s.cfg.Trx) == 0 {
		panic("Stage.Prepare called with zero trx")
	}

	// Test connection to MySQL
	dbconn.SetConfig(s.cfg.MySQL)
	db, dsnRedacted, err := dbconn.Make()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return err
	}
	db.Close() // test conn
	log.Printf("Connected to %s", dsnRedacted)

	// Load and validate all config.stage.trx files. This makes and validates all
	// data generators, too. Being valid means only that the Finch config/setup is
	// valid, not the SQL statements because those aren't run yet, so MySQL might
	// still return errors on Run.
	finch.Debug("load trx")
	trxSet, err := trx.Load(s.cfg.Trx, s.gds, s.cfg.Params)
	if err != nil {
		return err
	}

	// Allocate the workload (config.stage.workload): execution groups, client groups,
	// clients, and trx assigned to clients. This is done in two steps. First, Groups
	// returns the execution groups. Second, Clients returns the ready-to-run clients
	// for each exec group. Both steps are required but separated for testing because
	// the second is complex.
	finch.Debug("alloc clients")
	a := workload.Allocator{
		Stage:     s.cfg.N,
		StageName: s.cfg.Name,
		TrxSet:    trxSet,
		Workload:  s.cfg.Workload,
		StageQPS:  limit.NewRate(finch.Uint(s.cfg.QPS)), // nil if config.stage.qps == 0
		StageTPS:  limit.NewRate(finch.Uint(s.cfg.TPS)), // nil if config.stage.tps == 0
		DoneChan:  s.doneChan,
	}
	groups, err := a.Groups()
	if err != nil {
		return err
	}
	s.execGroups, err = a.Clients(groups, s.stats != nil)
	if err != nil {
		return err
	}

	// Initialize all clients in all exec groups, and register their stats with
	// the Collector
	finch.Debug("init clients")
	for egNo := range s.execGroups {
		for cgNo := range s.execGroups[egNo] {
			for _, c := range s.execGroups[egNo][cgNo].Clients {
				if err := c.Init(); err != nil {
					return err
				}
				if s.stats != nil {
					s.stats.Watch(c.Stats)
				}
			}
		}
	}

	return nil
}

func (s *Stage) Run(ctxFinch context.Context) {
	// There are 3 levels of contexts:
	//
	//   ctxFinch			from startup.Finch, catches CTRL-C
	//   └──ctxStage		config.stage.runtime, stage runtime
	//      └──ctxClients	config.stage.workload.runtime, client group runtime
	//
	// The ctxClients can end before the ctxStage if, for example, a client group
	// is conifgured to run for less than the full stage runtime. Different client
	// groups can also have different runtimes.
	var ctxStage context.Context
	var cancelStage context.CancelFunc
	if s.cfg.Runtime != "" {
		d, _ := time.ParseDuration(s.cfg.Runtime) // already validated
		ctxStage, cancelStage = context.WithDeadline(ctxFinch, time.Now().Add(d))
		defer cancelStage() // stage and all clients
		log.Printf("[%s] Running for %s", s.cfg.Name, s.cfg.Runtime)
	} else {
		ctxStage = ctxFinch
		log.Printf("[%s] Running (no runtime limit)", s.cfg.Name)
	}

	if s.stats != nil {
		s.stats.Start()
	}

	if finch.CPUProfile != nil {
		pprof.StartCPUProfile(finch.CPUProfile)
	}

	for egNo := range s.execGroups { // ------------------------------------- execution groups
		if ctxFinch.Err() != nil {
			break
		}
		nClients := 0
		for cgNo := range s.execGroups[egNo] { // --------------------------- client groups
			log.Printf("[%s] Execution group %d, client group %d, runnning %d clients", s.cfg.Name, egNo+1, cgNo+1, len(s.execGroups[egNo][cgNo].Clients))
			nClients += len(s.execGroups[egNo][cgNo].Clients)
			var ctxClients context.Context
			var cancelClients context.CancelFunc
			if s.execGroups[egNo][cgNo].Runtime > 0 {
				// Client group runtime (plus stage runtime, if any)
				finch.Debug("eg %d/%d exec %s", s.execGroups[egNo][cgNo].Runtime)
				ctxClients, cancelClients = context.WithDeadline(ctxStage, time.Now().Add(s.execGroups[egNo][cgNo].Runtime))
				defer cancelClients()
			} else {
				// Stage runtime limit, if any
				finch.Debug("%d/%d no limit", egNo, cgNo)
				ctxClients = ctxStage
			}
			for _, c := range s.execGroups[egNo][cgNo].Clients { // --------- clients
				go c.Run(ctxClients)
			}
		} // start all clients, then...

		clientErrors := make([]*client.Client, 0, nClients)
	CLIENTS:
		for nClients > 0 { // wait for clients
			select {
			case c := <-s.doneChan:
				finch.Debug("%s done: %v", c.RunLevel, c.Error)
				nClients -= 1
				if c.Error.Err != nil {
					clientErrors = append(clientErrors, c)
				}
			case <-ctxStage.Done():
				finch.Debug("stage runtime elapsed")
				break CLIENTS
			case <-ctxFinch.Done():
				finch.Debug("finch terminated")
				break CLIENTS
			}
		}
		if nClients > 0 {
			// spinWaitMs gives clients a _little_ time to finish when either
			// context is cancelled. This must be done to avoid a data race in
			// stats reporting: the CLIENTS loop finishes and stats are reported
			// below while a client is still writing to those stats. (This is
			// also due to fact that stats are lock-free.) So when a context is
			// cancelled, start sleeping 1ms and decrementing spinWaitMs which
			// lets this for loop continue (spin) but also timeout quickly.
			finch.Debug("spin wait for %d clients", nClients)
			spinWaitMs := 10
			for spinWaitMs > 0 && nClients > 0 {
				select {
				case c := <-s.doneChan:
					finch.Debug("%s done: %v", c.RunLevel, c.Error)
					nClients -= 1
					if c.Error.Err != nil {
						clientErrors = append(clientErrors, c)
					}
				default:
					time.Sleep(1 * time.Millisecond)
					spinWaitMs -= 1
				}
			}
		}
		if nClients > 0 {
			log.Printf("[%s] WARNING: %d clients did not stop, statistics are not accurate", s.cfg.Name, nClients)
		}
		if len(clientErrors) > 0 {
			log.Printf("%d client errors:\n", len(clientErrors))
			for _, c := range clientErrors {
				log.Printf("  %s: %s (%s)", c.RunLevel.ClientId(), c.Error.Err, c.Statements[c.Error.StatementNo].Query)
			}
		}
	}

	if finch.CPUProfile != nil {
		pprof.StopCPUProfile()
	}

	if s.stats != nil {
		if !s.stats.Stop(3*time.Second, ctxFinch.Err() != nil) {
			log.Printf("\n[%s] Timeout waiting for final statistics, reported values are incomplete", s.cfg.Name)
		}
	}
}
