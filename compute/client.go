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
	"github.com/square/finch/data"
	"github.com/square/finch/proto"
	"github.com/square/finch/stage"
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
	//for {
	c.gds.Reset() // keep data from globally-scoped generators; delete the rest
	if err := c.run(ctxFinch); err != nil {
		if ctxFinch.Err() != nil {
			return nil
		}
		log.Println(err)
		time.Sleep(2 * time.Second) // prevent uncontrolled error loop
	}
	//}
	return nil
}

func (c *Client) run(ctxFinch context.Context) error {
	// ------------------------------------------------------------------
	// Fetch stage fails (wait for GET /boot to return)
	var cfg config.Stage
	log.Printf("Waiting to boot from %s...", c.addr)
	c.client.PrintErrors = false
	_, body, err := c.client.Get(ctxFinch, "/boot", nil, proto.R{2 * time.Second, 1 * time.Second, -1})
	if err != nil {
		return err
	}
	c.client.PrintErrors = true
	if err := json.Unmarshal(body, &cfg); err != nil {
		return fmt.Errorf("cannot decode stage config file from server: %s", err)
	}
	stageName := cfg.Name
	c.client.StageId = cfg.Id
	defer func() { c.client.StageId = "" }()
	fmt.Printf("#\n# %s (%s)\n#\n", stageName, cfg.Id)

	// ----------------------------------------------------------------------
	// Fetch all stage and trx files from server, put in local temp dir
	tmpdir, err := os.MkdirTemp("", "finch")
	if err != nil {
		return fmt.Errorf("cannot make temp dir: %s", err)
	}
	if !finch.Debugging {
		defer os.RemoveAll(tmpdir)
	}
	finch.Debug("tmp dir: %s", tmpdir)
	if err := c.getTrxFiles(ctxFinch, cfg, tmpdir); err != nil {
		return err
	}

	// ------------------------------------------------------------------
	// Local boot and ack
	for k := range cfg.Stats.Report {
		if k == "stdout" {
			continue
		}
		delete(cfg.Stats.Report, k)
	}
	cfg.Stats.Report["server"] = map[string]string{
		"server":   c.addr,
		"client":   c.name,
		"stage-id": c.client.StageId,
	}
	stats, err := stats.NewCollector(cfg.Stats, c.name, 1)
	if err != nil {
		return err
	}

	log.Printf("[%s] Booting", stageName)
	local := stage.New(cfg, c.gds, stats)
	if err := local.Prepare(ctxFinch); err != nil {
		log.Printf("[%s] Boot error, notifying server: %s", stageName, err)
		c.client.Send(ctxFinch, "/boot", err.Error(), proto.R{500 * time.Millisecond, 100 * time.Millisecond, 3}) // don't care if this fails
		return err                                                                                                // return original error not Send error
	}

	// Boot ack; don't continue on error because we're no longer in sync with server
	log.Printf("[%s] Boot successful, notifying server", stageName)
	if err := c.client.Send(ctxFinch, "/boot", nil, proto.R{1 * time.Second, 100 * time.Millisecond, 10}); err != nil {
		log.Printf("[%s] Sending book ack to server failed: %s", stageName, err)
		return err
	}

	// ----------------------------------------------------------------------
	// Wait for run signal. This might be a little while if server is for
	// other remote instances.
	log.Printf("[%s] Waiting for run signal", stageName)
	resp, _, err := c.client.Get(ctxFinch, "/run", nil, proto.R{60 * time.Second, 100 * time.Millisecond, 3})
	if err != nil {
		log.Printf("[%s] Timeout waiting for run signal after successful boot, giving up (is the server offline?)", stageName)
		return err
	}
	if resp.StatusCode == http.StatusResetContent {
		log.Printf("[%s] Boot test successful", stageName)
		return nil
	}

	// ----------------------------------------------------------------------
	// Local run and ack
	ctxRun, cancelRun := context.WithCancel(ctxFinch)
	doneChan := make(chan struct{})
	defer close(doneChan)
	lostServer := false
	stageDone := false
	go func() {
		defer cancelRun()
		for {
			time.Sleep(1 * time.Second)
			select {
			case <-doneChan:
				finch.Debug("stop check goroutine stopped")
				return
			default:
			}
			resp, _, err := c.client.Get(ctxFinch, "/ping", nil, proto.R{500 * time.Millisecond, 100 * time.Millisecond, 5})
			if err != nil {
				log.Printf("[%s] Lost contact with server while running, aborting", stageName)
				lostServer = true
				return
			}
			if resp.StatusCode == http.StatusResetContent {
				stageDone = true
				log.Printf("[%s] Server stopped stage", stageName)
				return
			}
		}
	}()

	log.Printf("[%s] Running", stageName)
	if err := local.Run(ctxRun); err != nil {
		log.Printf("[%s] Run stopped: %v (lost server:%v stage stopped:%v); sending done signal to server (5s timeout)", stageName, err, lostServer, stageDone)
	} else {
		log.Printf("[%s] Completed successfully; sending done signal to server (5s timeout)", stageName)
	}

	// Run ack; ok if this fails because we're done, nothing left to sync with server
	ctxDone, ctxCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer ctxCancel()
	if err := c.client.Send(ctxDone, "/run", err, proto.R{500 * time.Millisecond, 100 * time.Millisecond, 3}); err != nil {
		log.Printf("[%s] Sending done signal to server failed, ignoring: %s", stageName, err)
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
		resp, body, err := c.client.Get(ctxFinch, "/file", ref, proto.R{5 * time.Second, 100 * time.Millisecond, 3})
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
		finch.Debug("wrote %s", filename)
		trx[i].File = filename
	}
	return nil
}
