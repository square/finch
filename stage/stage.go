// Copyright 2023 Block, Inc.

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

	whyDone := "clients completed"
EXEC_GROUPS:
	for egNo := range s.execGroups { // ------------------------------------- execution groups
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

		for nClients > 0 { // wait for clients
			select {
			case c := <-s.doneChan:
				nClients -= 1
				if c.Error != nil {
					log.Printf("[%s] Client %s failed: %v", s.cfg.Name, c.RunLevel.ClientId(), c.Error)
				} else {
					finch.Debug("client done: %s", c.RunLevel.ClientId())
				}
			case <-ctxStage.Done():
				whyDone = "stage runtime elapsed"
				break EXEC_GROUPS
			case <-ctxFinch.Done():
				whyDone = "Finch terminated"
				break EXEC_GROUPS
			}
		}
	}

	if finch.CPUProfile != nil {
		pprof.StopCPUProfile()
	}

	if s.stats != nil {
		s.stats.Stop()
		finch.Debug("wait for final stats")
		timeout := time.After(5 * time.Second)
		select {
		case <-s.stats.Done():
		case <-timeout:
			s.stats.Report()
			log.Printf("\n[%s] Timeout waiting for final stats, forced final report (some stats might be lost)\n", s.cfg.Name)
		}
	}

	log.Printf("[%s] Stage done because %s\n", s.cfg.Name, whyDone)
}
