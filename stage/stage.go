// Copyright 2022 Block, Inc.

package stage

import (
	"context"
	"log"
	"time"

	"github.com/square/finch"
	"github.com/square/finch/client"
	"github.com/square/finch/config"
	"github.com/square/finch/data"
	"github.com/square/finch/limit"
	"github.com/square/finch/stats"
	"github.com/square/finch/trx"
	"github.com/square/finch/workload"
)

// A stage runs clients to execute events. Each client is identical.
// Stage runs the workload, controlling the order (by subset, if any).
type Stage struct {
	cfg   config.Stage
	scope *data.Scope
	stats *stats.Collector
	// --
	doneChan   chan *client.Client      // <-Client.Run()
	execGroups [][]workload.ClientGroup // [n][Client]
}

func New(cfg config.Stage, s *data.Scope, c *stats.Collector) *Stage {
	return &Stage{
		cfg:   cfg,
		scope: s,
		stats: c,
		// --
		doneChan: make(chan *client.Client, 1),
	}
}

func (s *Stage) Prepare() error {
	if len(s.cfg.Trx) == 0 {
		panic("Stage.Prepare called with zero trx")
	}
	log.Printf("Preparing stage %s\n", s.cfg.Name)

	// Load and validate all config.stage.trx files. This makes and validates all
	// data generators, too. Being valid means only that the Finch config/setup is
	// valid, not the SQL statements because those aren't run yet, so MySQL might
	// still return errors on Run.
	trxSet, err := trx.Load(s.cfg.Trx, s.scope)
	if err != nil {
		return err
	}

	// Allocate the workload (config.stage.workload): execution groups, client groups,
	// clients, and trx assigned to clients. This is done in two steps. First, Groups
	// returns the execution groups. Second, Clients returns the ready-to-run clients
	// for each exec group. Both steps are required but separated for testing because
	// the second is complex.
	a := workload.Allocator{
		Stage:    s.cfg.Name,
		TrxSet:   trxSet,
		Workload: s.cfg.Workload,
		StageQPS: limit.NewRate(s.cfg.QPS), // nil if config.stage.qps == 0
		StageTPS: limit.NewRate(s.cfg.TPS), // nil if config.stage.tps == 0
		DoneChan: s.doneChan,
	}
	groups, err := a.Groups()
	if err != nil {
		return err
	}
	s.execGroups, err = a.Clients(groups)
	if err != nil {
		return err
	}

	// Initialize all clients in all exec groups, and register their stats with
	// the Collector
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
		log.Printf("Running %s for %s", s.cfg.Name, s.cfg.Runtime)
	} else {
		ctxStage = ctxFinch
		log.Printf("Running %s (no runtime limit)", s.cfg.Name)
	}

	if s.stats != nil {
		s.stats.Start()
	}

EXEC_GROUPS:
	for i := range s.execGroups {
		nClients := 0
		for j := range s.execGroups[i] {
			log.Printf("Execution group %d, client group %d, runnning %d clients", i+1, j+1, len(s.execGroups[i][j].Clients))
			nClients += len(s.execGroups[i][j].Clients)

			// See comment block above ctxStage
			var ctxClients context.Context
			var cancelClients context.CancelFunc
			if s.execGroups[i][j].Runtime > 0 {
				finch.Debug("%d/%d exec %s", s.execGroups[i][j].Runtime)
				ctxClients, cancelClients = context.WithDeadline(ctxStage, time.Now().Add(s.execGroups[i][j].Runtime))
				defer cancelClients()
			} else {
				finch.Debug("%d/%d no limit")
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
				log.Printf("Client %s done (error: %v), %d still running", c.RunLevel.ClientId(), c.Error, nClients)
			case <-ctxStage.Done():
				log.Println("Stage runtime elapsed")
				break EXEC_GROUPS
			case <-ctxFinch.Done():
				log.Println("Finch terminated")
				break EXEC_GROUPS
			}
		}
	}

	if s.stats != nil {
		s.stats.Stop()
	}

	return nil
}
