// Copyright 2023 Block, Inc.

package compute

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/square/finch"
	"github.com/square/finch/config"
	"github.com/square/finch/proto"
	"github.com/square/finch/stats"
)

var RetryWait = 500 * time.Millisecond
var MaxTries = 100

// Client is a remote Instance that sends everything to the --server specified
// on the command line. The client handles client-server communication, and it
// wraps a Local that runs stages locally.
type Client struct {
	name   string
	addr   string
	local  *Local
	client *proto.Client
	tmpdir string
	stats  *stats.Collector
}

var _ Compute = &Client{}

func NewClient(name, addr string) *Client {
	return &Client{
		name:   name,
		addr:   strings.TrimSuffix(addr, "/"),
		client: proto.NewClient(name, addr),
	}
}

func (comp *Client) Stop() {
	if comp.local != nil {
		comp.local.Stop()
	}
}

func (comp *Client) Boot(ctx context.Context, _ config.File) error {
	// Fetch config file from remote server
	var cfg config.File
	log.Printf("Fetching config file from %s...", comp.addr)
	_, body, err := comp.client.Get(ctx, "/boot", nil)
	if err != nil {
		return err // Get retries so error is final
	}
	if err := json.Unmarshal(body, &cfg); err != nil {
		return fmt.Errorf("cannot decode config.File struct from server: %s", err)
	}

	// Fetch workload files from remote server if they don't exist locally
	dir, err := os.MkdirTemp("", "finch")
	if err != nil {
		return err
	}
	log.Printf("Tmp dir for stage files: %s", dir)
	comp.tmpdir = dir
	if err := comp.getTrxFiles(ctx, finch.STAGE_SETUP, cfg.Setup.Trx); err != nil {
		return err
	}
	if err := comp.getTrxFiles(ctx, finch.STAGE_WARMUP, cfg.Warmup.Trx); err != nil {
		return err
	}
	if err := comp.getTrxFiles(ctx, finch.STAGE_BENCHMARK, cfg.Benchmark.Trx); err != nil {
		return err
	}
	if err := comp.getTrxFiles(ctx, finch.STAGE_CLEANUP, cfg.Cleanup.Trx); err != nil {
		return err
	}

	// Create and boot local instance
	for k := range cfg.Stats.Report {
		if k == "stdout" {
			continue
		}
		delete(cfg.Stats.Report, k)
	}
	cfg.Stats.Report["server"] = map[string]string{
		"server": comp.addr,
		"client": comp.name,
	}
	comp.stats, err = stats.NewCollector(cfg.Stats, comp.name, 1)
	if err != nil {
		if !finch.Debugging {
			os.RemoveAll(comp.tmpdir)
		}
		comp.client.Error(err)
		return err
	}
	comp.local = NewLocal(comp.name, comp.stats)
	if err := comp.local.Boot(ctx, cfg); err != nil {
		if !finch.Debugging {
			os.RemoveAll(comp.tmpdir)
		}
		comp.client.Error(err)
		return err
	}

	// Notify server that we're ready to run
	log.Println("Sending boot signal")
	if err := comp.client.Send(ctx, "/boot", nil); err != nil {
		return err // Send retries so error is final
	}

	return nil
}

func (comp *Client) Run(ctx context.Context) error {
	if !finch.Debugging {
		defer os.RemoveAll(comp.tmpdir)
	}

	defer func() {
		log.Println("Sending stop signal")
		comp.client.Send(ctx, "/stop", nil) // Send retries
	}()

	// Contact remote server
	prevStageName := ""
	for {
		log.Println("Waiting for run signal")
		resp, body, err := comp.client.Get(ctx, "/run", nil)
		if err != nil {
			return err // Get retires so error is final
		}

		if resp.StatusCode == http.StatusNoContent {
			log.Println("Server reports done")
			return nil
		}

		stageName := string(body)
		if stageName == prevStageName {
			log.Println("Waiting for new stage to start...")
			time.Sleep(RetryWait)
			continue
		}

		log.Printf("Running stage %s", stageName)
		err = comp.local.Run(ctx, stageName)
		if err != nil {
			log.Printf("Error running stage %s: %s", stageName, err)
			log.Println("Sending error signal")
			comp.client.Error(err)
		}

		log.Println("Sending stage-done signal")
		if err := comp.client.Send(ctx, "/run", nil); err != nil {
			log.Fatal(err) // Send retries so error is fatal
		}

		prevStageName = stageName
	}
}

func (comp *Client) getTrxFiles(ctx context.Context, stage string, trx []config.Trx) error {
	if len(trx) == 0 {
		finch.Debug("stage %s has no trx, ignoring", stage)
		return nil
	}

	for i := range trx {
		if config.FileExists(trx[i].File) {
			log.Printf("Have local stage %s file %s; not fetching from server", stage, trx[i].File)
			continue
		}
		log.Printf("Fetching stage %s file %s...", stage, trx[i].File)
		ref := [][]string{
			{"stage", stage},
			{"i", fmt.Sprintf("%d", i)},
		}
		resp, body, err := comp.client.Get(ctx, "/file", ref)
		if err != nil {
			return err // Get retries so error is final
		}
		finch.Debug("%+v", resp)

		filename := filepath.Join(comp.tmpdir, filepath.Base(trx[i].File))
		f, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE, 0440)
		if err != nil {
			return err
		}
		if _, err := f.Write(body); err != nil {
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
		log.Printf("Wrote %s", filename)
		trx[i].File = filename
	}

	return nil
}
