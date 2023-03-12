// Copyright 2022 Block, Inc.

package compute

import (
	"context"
	"log"
	"time"

	"github.com/square/finch"
	"github.com/square/finch/config"
	"github.com/square/finch/data"
	"github.com/square/finch/dbconn"
	"github.com/square/finch/stage"
	"github.com/square/finch/stats"
)

// Instance runs stages locally. It's controlled by a local or remote coordinator.
type Instance struct {
	name   string
	stages map[string]*stage.Stage
	stats  *stats.Collector
}

func NewInstance(name string, r *stats.Collector) *Instance {
	return &Instance{
		name:   name,
		stages: map[string]*stage.Stage{},
		stats:  r,
	}
}

func (comp *Instance) Stop() {
}

func (comp *Instance) Boot(ctx context.Context, cfg config.File) error {
	// Test connection to MySQL
	dbconn.SetFactory(cfg.MySQL, nil)
	db, dsnRedacted, err := dbconn.Make()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return err
	}
	db.Close()
	log.Printf("Connected to %s", dsnRedacted)

	// //////////////////////////////////////////////////////////////////////
	// Prepare stages
	// //////////////////////////////////////////////////////////////////////

	global := data.NewScope()

	if !cfg.Setup.Disable {
		stage := stage.New(cfg.Setup, global, nil)
		if err := stage.Prepare(); err != nil {
			return err
		}
		comp.stages[finch.STAGE_SETUP] = stage
		global.Reset() // keep global scope data generators; delete the rest
	}

	if !cfg.Warmup.Disable {
		stage := stage.New(cfg.Warmup, global, nil)
		if err := stage.Prepare(); err != nil {
			return err
		}
		comp.stages[finch.STAGE_WARMUP] = stage
		global.Reset()
	}

	if !cfg.Benchmark.Disable {
		stage := stage.New(cfg.Benchmark, global, comp.stats)
		if err := stage.Prepare(); err != nil {
			return err
		}
		comp.stages[finch.STAGE_BENCHMARK] = stage
		global.Reset()
	}

	if !cfg.Cleanup.Disable {
		stage := stage.New(cfg.Cleanup, global, nil)
		if err := stage.Prepare(); err != nil {
			return err
		}
		comp.stages[finch.STAGE_CLEANUP] = stage
	}

	return nil
}

func (comp *Instance) Run(ctx context.Context, stageName string) error {
	stage, ok := comp.stages[stageName]
	if !ok {
		return nil
	}

	// If script, run that instead of stage
	//if cfg.Script != "" {
	//	continue
	//}

	// Run stage
	return stage.Run(ctx)
}
