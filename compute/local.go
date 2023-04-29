// Copyright 2023 Block, Inc.

package compute

import (
	"context"
	"log"
	"time"

	"github.com/square/finch/config"
	"github.com/square/finch/data"
	"github.com/square/finch/dbconn"
	"github.com/square/finch/stage"
	"github.com/square/finch/stats"
)

type Local struct {
	name  string
	gds   *data.Scope
	stats *stats.Collector
	// --
	stage *stage.Stage
}

func NewLocal(name string, gds *data.Scope, stats *stats.Collector) *Local {
	return &Local{
		name:  name,
		gds:   gds,
		stats: stats,
	}
}

func (in *Local) Boot(ctxFinch context.Context, cfg config.Stage) error {
	// Test connection to MySQL
	dbconn.SetFactory(cfg.MySQL, nil)
	db, dsnRedacted, err := dbconn.Make()
	if err != nil {
		return err
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(ctxFinch, 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return err
	}
	log.Printf("Connected to %s", dsnRedacted)

	// Prepare stage
	in.stage = stage.New(cfg, in.gds, in.stats)
	return in.stage.Prepare()
}

func (in *Local) Run(ctxFinch context.Context) error {
	return in.stage.Run(ctxFinch)
}
