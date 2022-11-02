// Copyright 2022 Block, Inc.

package compute

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/square/finch"
	"github.com/square/finch/config"
	"github.com/square/finch/stats"
)

var RetryWait = 500 * time.Millisecond
var MaxTries = 100

type Remote struct {
	name   string
	addr   string
	local  *Instance
	client *http.Client
	tmpdir string
	ag     *stats.Ag
}

var _ Coordinator = &Remote{}

func NewRemote(name, addr string) *Remote {
	return &Remote{
		name:   name,
		addr:   strings.TrimSuffix(addr, "/"),
		client: finch.MakeHTTPClient(),
	}
}

func (comp *Remote) Stop() {
	if comp.local != nil {
		comp.local.Stop()
	}
}

func (comp *Remote) Boot(_ config.File) error {
	// Fetch config file from remote server
	log.Printf("Fetching config file from %s...", comp.addr)
	var cfg config.File
	printErr := true
RETRY:
	for {
		resp, err := comp.client.Get(comp.url("/boot", nil))
		if err != nil {
			if printErr {
				log.Println(err)
				printErr = false // don't spam output
			}
			time.Sleep(RetryWait)
			continue RETRY
		}
		printErr = true
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			if printErr {
				log.Printf("%s: %s", err, string(body))
				printErr = false // don't spam output
			}
			time.Sleep(RetryWait)
			continue RETRY
		}
		printErr = true

		if err := json.Unmarshal(body, &cfg); err != nil {
			if printErr {
				log.Printf("%s: %s", err, string(body))
				printErr = false // don't spam output
			}
			time.Sleep(RetryWait)
			continue RETRY
		}

		break // success
	}

	// Fetch workload files from remote server if they don't exist locally
	dir, err := os.MkdirTemp("", "finch")
	if err != nil {
		return err
	}
	comp.tmpdir = dir
	comp.getTrxFiles(finch.STAGE_SETUP, cfg.Setup.Workload)
	comp.getTrxFiles(finch.STAGE_WARMUP, cfg.Warmup.Workload)
	comp.getTrxFiles(finch.STAGE_BENCHMARK, cfg.Benchmark.Workload)
	comp.getTrxFiles(finch.STAGE_CLEANUP, cfg.Cleanup.Workload)

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
	comp.ag, err = stats.NewAg(1, cfg.Stats)
	if err != nil {
		os.RemoveAll(comp.tmpdir)
		comp.err(err)
		return err
	}
	comp.local = NewInstance(
		comp.name,
		stats.NewCollector(cfg.Stats, comp.name, comp.ag.Chan()),
	)
	if err := comp.local.Boot(cfg); err != nil {
		os.RemoveAll(comp.tmpdir)
		comp.err(err)
		return err
	}

	// Notify server that we're ready to run
	if _, err := comp.client.Post(comp.url("/boot", nil), "text/plain", nil); err != nil {
		log.Fatal(err)
	}
	return nil
}

func (comp *Remote) Run(ctx context.Context) error {
	defer os.RemoveAll(comp.tmpdir)

	defer func() {
		for i := 0; i < 3; i++ {
			if _, err := comp.client.Post(comp.url("/stop", nil), "text/plain", nil); err != nil {
				log.Println(err)
				time.Sleep(RetryWait)
				continue
			}
			return
		}
	}()

	// Contact remote server
	printErr := true
	prevStageName := ""
	for {
		finch.Debug("get stage from server")
		resp, err := comp.client.Get(comp.url("/run", nil))
		if err != nil {
			if printErr {
				log.Println(err)
				printErr = false // don't spam output
			}
			time.Sleep(RetryWait)
			continue
		}
		printErr = true

		if resp.StatusCode == http.StatusNoContent {
			log.Println("Server reports done")
			return nil
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			if printErr {
				log.Printf("%s: %s", err, string(body))
				printErr = false // don't spam output
			}
			time.Sleep(RetryWait)
			continue
		}
		resp.Body.Close()
		stageName := string(body)

		if stageName == prevStageName {
			log.Println("Waiting for new stage to start...")
			time.Sleep(RetryWait)
			continue
		}

		// Relay to local
		// stage.Run will log that it's running
		if stageName == finch.STAGE_BENCHMARK {
			go comp.ag.Run()
		}
		err = comp.local.Run(ctx, stageName)
		if err != nil {
			errMsg := bytes.NewBufferString(err.Error())
			if _, err := comp.client.Post(comp.url("/run", nil), "text/plain", errMsg); err != nil {
				log.Fatal(err)
			}
		}
		if stageName == finch.STAGE_BENCHMARK {
			comp.ag.Done()
		}
		if _, err := comp.client.Post(comp.url("/run", nil), "text/plain", nil); err != nil {
			log.Fatal(err)
		}

		prevStageName = stageName
	}
}

// --------------------------------------------------------------------------

func (comp *Remote) getTrxFiles(stage string, trx []config.Trx) error {
	if len(trx) == 0 {
		return nil
	}

	for i := range trx {
		log.Printf("Fetching stage %s file %s...", stage, trx[i].File)
		if config.FileExists(trx[i].File) {
			continue
		}
		ref := [][]string{
			{"stage", stage},
			{"i", fmt.Sprintf("%d", i)},
		}
		printErr := true
		var body []byte
	RETRY:
		for {
			resp, err := comp.client.Get(comp.url("/file", ref))
			if err != nil {
				if printErr {
					log.Println(err)
					printErr = false // don't spam output
				}
				time.Sleep(RetryWait)
				continue RETRY
			}
			printErr = true
			defer resp.Body.Close()

			body, err = io.ReadAll(resp.Body)
			if err != nil {
				if printErr {
					log.Printf("%s: %s", err, string(body))
					printErr = false // don't spam output
				}
				time.Sleep(RetryWait)
				continue RETRY
			}
			break // success
		}
		filename := filepath.Join(comp.tmpdir, filepath.Base(trx[i].File))
		f, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE, 0755)
		if err != nil {
			return err
		}
		if _, err := f.Write(body); err != nil {
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
		trx[i].File = filename
	}

	return nil
}

func (comp *Remote) url(path string, params [][]string) string {
	// Every request requires 'name=...' to tell server this client's name.
	// It's not a hostname, just a user-defined name for the remote compute instance.
	u := comp.addr + path + "?name=" + url.QueryEscape(comp.name)
	if len(params) > 0 {
		escaped := make([]string, len(params))
		for i := range params {
			escaped[i] = params[i][0] + "=" + url.QueryEscape(params[i][1])
		}
		u += strings.Join(escaped, "&")
	}
	finch.Debug("%s", u)
	return u
}

func (comp *Remote) err(err error) {
	errString := bytes.NewBufferString(err.Error())
	printErr := true
RETRY:
	for i := 0; i < MaxTries; i++ {
		_, err := comp.client.Post(comp.url("/error", nil), "text/plain", errString)
		if err != nil {
			if printErr {
				log.Println(err)
				printErr = false // don't spam output
			}
			time.Sleep(RetryWait)
			continue RETRY
		}
		break
	}
}
