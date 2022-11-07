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
	"github.com/square/finch/limit"
	"github.com/square/finch/stats"
	"github.com/square/finch/workload"
)

// A stage runs clients to execute events. Each client is identical.
// Stage runs the workload, controlling the order (by subset, if any).
type Stage struct {
	cfg config.Stage
	// --
	clients  [][]*client.Client // [seqNo][Client...]
	doneChan chan error         // <-Client.Run()
	stats    *stats.Collector
}

func New(cfg config.Stage, r *stats.Collector) *Stage {
	return &Stage{
		cfg:      cfg,
		doneChan: make(chan error, 1),
		stats:    r,
	}
}

func (s *Stage) Prepare() error {
	if s.cfg.Disable {
		finch.Debug("stage %s disabled", s.cfg.Name)
		return nil
	}
	log.Printf("Preparing stage %s\n", s.cfg.Name)

	// Load workload
	order, trx, err := workload.Transactions(s.cfg.Workload)
	if err != nil {
		return err
	}
	if len(trx) == 0 {
		return fmt.Errorf("no statements")
	}

	// Allocate client groups based on sequence
	a := client.AllocateArgs{
		Stage:          s.cfg.Name,
		ClientGroups:   s.cfg.Clients,
		AutoAllocation: !s.cfg.DisableAutoAllocation,
		Trx:            trx,
		WorkloadOrder:  order,
		Sequence:       s.cfg.Sequence,
		DoneChan:       s.doneChan,
		StageQPS:       limit.NewRate(s.cfg.QPS), // nil if config.stage.qps == 0
		StageTPS:       limit.NewRate(s.cfg.TPS), // nil if config.stage.tps == 0
	}
	s.clients, err = client.Allocate(a)
	if err != nil {
		return err
	}

	// Prepare clients (e.g. create prepared statement handle) and register
	// with stats.Collector
	for seqNo := range s.clients {
		for _, c := range s.clients[seqNo] {
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
	for seqNo := range s.clients {
		log.Printf("Sequence %d, %d clients", seqNo, len(s.clients[seqNo]))
		for _, client := range s.clients[seqNo] {
			go client.Run(ctx)
		}

		nRunning := len(s.clients[seqNo])
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
