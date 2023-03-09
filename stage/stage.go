// Copyright 2022 Block, Inc.

package stage

import (
	"context"
	"fmt"
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
	doneChan chan error // <-Client.Run()
	clients  [][]*client.Client
}

func New(cfg config.Stage, s *data.Scope, r *stats.Collector) *Stage {
	return &Stage{
		cfg:   cfg,
		scope: s,
		stats: r,
		// --
		doneChan: make(chan error, 1),
	}
}

func (s *Stage) Prepare() error {
	if s.cfg.Disable {
		finch.Debug("stage %s disabled", s.cfg.Name)
		return nil
	}
	if len(s.cfg.Trx) == 0 {
		finch.Debug("stage %s disabled", s.cfg.Name)
		return nil
	}
	log.Printf("Preparing stage %s\n", s.cfg.Name)

	// Load trx set
	trxSet, err := trx.Load(s.cfg.Trx, s.scope)
	if err != nil {
		return err
	}
	for _, trxName := range trxSet.Order {
		if len(trxSet.Statements[trxName]) == 0 {
			return fmt.Errorf("transactions %s has zero statements, at least 1 required", trxName)
		}
	}

	// Allocate client groups based on sequence
	a := workload.Allocator{
		Stage:      s.cfg.Name,
		TrxSet:     trxSet,
		ExecGroups: s.cfg.Workload,
		ExecMode:   s.cfg.Exec,
		StageQPS:   limit.NewRate(s.cfg.QPS), // nil if config.stage.qps == 0
		StageTPS:   limit.NewRate(s.cfg.TPS), // nil if config.stage.tps == 0
		DoneChan:   s.doneChan,
		Auto:       !s.cfg.DisableAutoAllocation,
	}
	groups, err := a.Groups()
	if err != nil {
		return err
	}
	s.clients, err = a.Clients(groups)
	if err != nil {
		return err
	}

	// Prepare clients (e.g. create prepared statement handle) and register
	// with stats.Collector
	for e := range s.clients {
		for _, c := range s.clients[e] {
			if err := c.Prepare(); err != nil {
				return err
			}
			if s.stats != nil {
				s.stats.Watch(c.Stats)
			}
		}
	}

	return nil
}

func (s *Stage) Run(ctxServer context.Context) error {
	var ctx context.Context
	var cancel context.CancelFunc
	if s.cfg.Runtime != "" {
		d, _ := time.ParseDuration(s.cfg.Runtime) // already validated
		ctx, cancel = context.WithDeadline(ctxServer, time.Now().Add(d))
		defer cancel() // stage and all clients
		log.Printf("Running %s for %s", s.cfg.Name, s.cfg.Runtime)
	} else {
		ctx = ctxServer
		log.Printf("Running %s (no runtime limit)", s.cfg.Name)
	}

	if s.stats != nil {
		s.stats.Start()
	}

CLIENTS:
	for e := range s.clients {
		log.Printf("Execution group %d, %d clients", e, len(s.clients[e]))
		for _, c := range s.clients[e] {
			go c.Run(ctx)
		}

		nRunning := len(s.clients[e])
		for nRunning > 0 {
			// Wait for clients
			select {
			case <-ctx.Done():
				log.Println("Runtime elapsed")
				break CLIENTS
			case err := <-s.doneChan:
				nRunning -= 1
				if err != nil {
					log.Printf("Client error: %v", err)
				} else {
					log.Printf("Client finished, %d still running", nRunning)
				}
			}
		}

	}

	if s.stats != nil {
		time.Sleep(250 * time.Millisecond)
		s.stats.Stop()
	}

	return nil
}
