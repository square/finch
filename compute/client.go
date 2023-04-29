// Copyright 2023 Block, Inc.

package compute

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/square/finch"
	"github.com/square/finch/config"
	"github.com/square/finch/data"
	"github.com/square/finch/proto"
	"github.com/square/finch/stats"
)

// Client is a remote Instance that sends everything to the --server specified
// on the command line. The client handles client-server communication, and it
// wraps a Local that runs stages locally.
type Client struct {
	name string
	addr string
	// --
	gds    *data.Scope
	client *proto.Client
}

func NewClient(name, addr string) *Client {
	if !strings.HasPrefix(addr, "http://") {
		addr = "http://" + addr
	}

	return &Client{
		name: name,
		addr: strings.TrimSuffix(addr, "/"),
		// --
		gds:    data.NewScope(),
		client: proto.NewClient(name, addr),
	}
}

func (c *Client) Run(ctxFinch context.Context) error {
	defer func() {
		log.Println("Sending stop signal")
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		c.client.Send(ctx, "/stop", nil) // Send retries
		cancel()
	}()

	for {
		c.gds.Reset() // keep data from globally-scoped generators; delete the rest
		if err := c.run(ctxFinch); err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
			log.Println(err)
			time.Sleep(1 * time.Second)
		}
	}
}

func (c *Client) run(ctxFinch context.Context) error {
	// Fetch config file from remote server
	var cfg config.Stage
	log.Printf("Waiting to boot from %s...", c.addr)
	_, body, err := c.client.Get(ctxFinch, "/boot", nil)
	if err != nil {
		return err // Get retries so error is final
	}
	if err := json.Unmarshal(body, &cfg); err != nil {
		return fmt.Errorf("cannot decode config.File struct from server: %s", err)
	}

	// Fetch workload files from remote server if they don't exist locally
	tmpdir, err := os.MkdirTemp("", "finch")
	if err != nil {
		return err
	}
	if !finch.Debugging {
		defer os.RemoveAll(tmpdir)
	}
	log.Printf("Tmp dir for stage files: %s", tmpdir)

	if err := c.getTrxFiles(ctxFinch, cfg, tmpdir); err != nil {
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
		"server": c.addr,
		"client": c.name,
	}
	stats, err := stats.NewCollector(cfg.Stats, c.name, 1)
	if err != nil {
		if !finch.Debugging {
			os.RemoveAll(tmpdir)
		}
		c.client.Error(err)
		return err
	}

	local := NewLocal(c.name, c.gds, stats)
	if err := local.Boot(ctxFinch, cfg); err != nil {
		if !finch.Debugging {
			os.RemoveAll(tmpdir)
		}
		c.client.Error(err)
		return err
	}

	// Notify server that we're ready to run
	log.Println("Sending boot signal")
	if err := c.client.Send(ctxFinch, "/boot", nil); err != nil {
		return err // Send retries so error is final
	}

	log.Println("Waiting for run signal")
	_, body, err = c.client.Get(ctxFinch, "/run", nil)
	if err != nil {
		return err // Get retires so error is final
	}
	//if resp.StatusCode == http.StatusNoContent {

	stageName := cfg.Name // shortcut
	log.Printf("Running stage %s", stageName)
	if err := local.Run(ctxFinch); err != nil {
		log.Printf("Error running stage %s: %s", stageName, err)
		log.Println("Sending error signal")
		c.client.Error(err)
	}

	log.Println("Sending stage-done signal")
	if err := c.client.Send(ctxFinch, "/run", nil); err != nil {
		log.Fatal(err) // Send retries so error is fatal
	}

	return nil
}

func (c *Client) getTrxFiles(ctxFinch context.Context, cfg config.Stage, tmpdir string) error {
	trx := cfg.Trx
	for i := range trx {
		if config.FileExists(trx[i].File) {
			log.Printf("Have local stage %s file %s; not fetching from server", cfg.Name, trx[i].File)
			continue
		}
		log.Printf("Fetching stage %s file %s...", cfg.Name, trx[i].File)
		ref := [][]string{
			{"stage", cfg.Name},
			{"i", fmt.Sprintf("%d", i)},
		}
		resp, body, err := c.client.Get(ctxFinch, "/file", ref)
		if err != nil {
			return err // Get retries so error is final
		}
		finch.Debug("%+v", resp)

		filename := filepath.Join(tmpdir, filepath.Base(trx[i].File))
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
