// Copyright 2023 Block, Inc.

package stage

import (
	"context"
	"log"
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

// A stage runs clients to execute events. Each client is identical.
// Stage runs the workload, controlling the order (by subset, if any).
type Stage struct {
	cfg   config.Stage
	gds   *data.Scope
	stats *stats.Collector
	// --
	doneChan       chan *client.Client      // <-Client.Run()
	execGroups     [][]workload.ClientGroup // [n][Client]
	runtimeLimited bool                     // true is run time is limited
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
	dbconn.SetFactory(s.cfg.MySQL, nil)
	db, dsnRedacted, err := dbconn.Make()
	if err != nil {
		return err
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return err
	}
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
	for i := range s.execGroups {
		for j := range s.execGroups[i] {
			for _, c := range s.execGroups[i][j].Clients {
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

func (s *Stage) Run(ctxFinch context.Context) error {
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

	terminated := false
EXEC_GROUPS:
	for i := range s.execGroups {
		nClients := 0
		for j := range s.execGroups[i] {
			log.Printf("[%s] Execution group %d, client group %d, runnning %d clients", s.cfg.Name, i+1, j+1, len(s.execGroups[i][j].Clients))
			nClients += len(s.execGroups[i][j].Clients)

			// See comment block above ctxStage
			var ctxClients context.Context
			var cancelClients context.CancelFunc
			if s.execGroups[i][j].Runtime > 0 {
				finch.Debug("eg %d/%d exec %s", s.execGroups[i][j].Runtime)
				ctxClients, cancelClients = context.WithDeadline(ctxStage, time.Now().Add(s.execGroups[i][j].Runtime))
				defer cancelClients()
			} else {
				finch.Debug("%d/%d no limit", i, j)
				ctxClients = ctxStage
			}

			// Run all clients from <- client grp (j) <- exec grp (i)
			for _, c := range s.execGroups[i][j].Clients {
				go c.Run(ctxClients)
			}
		}

		for nClients > 0 {
			select {
			case c := <-s.doneChan:
				nClients -= 1
				if c.Error != nil {
					log.Printf("[%s] Client %s failed: %v", s.cfg.Name, c.RunLevel.ClientId(), c.Error)
				} else {
					if nClients > 0 {
						log.Printf("[%s] Client done, %d still running", s.cfg.Name, nClients)
					} else {
						log.Printf("[%s] Client done", s.cfg.Name)
					}
				}
			case <-ctxStage.Done():
				finch.Debug("runtime elapsed")
				break EXEC_GROUPS
			case <-ctxFinch.Done():
				finch.Debug("finch terminated")
				terminated = true
				break EXEC_GROUPS
			}
		}
	}

	// Wait for and report final stats if there are stats and it's _not_ the case
	// that user ternmainted Finch early (CTRL-C) without periodic stats, which
	// would result in terminating without any stats. Meaning, with stats.freq==0,
	// we want final on CTRL-C because these will be the only stats. But with
	// freq > 0, we don't need final stats on CTRL-C because, presumably, user got
	// some periodic stats already and they're terminating when they've gotten
	// enough output.
	if s.stats != nil && !(terminated && s.stats.Freq > 0) {
		finch.Debug("wait for final stats")
		s.stats.Stop()
		timeout := time.After(5 * time.Second)
		select {
		case <-s.stats.Done():
		case <-timeout:
			log.Println("[%s] Timeout waiting for final stats, forcing final report (some stats might be lost)", s.cfg.Name)
			s.stats.Report()
		}
	}

	return nil
}
