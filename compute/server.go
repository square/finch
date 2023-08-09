// Copyright 2023 Block, Inc.

package compute

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/rs/xid"

	"github.com/square/finch"
	"github.com/square/finch/config"
	"github.com/square/finch/data"
	"github.com/square/finch/stage"
	"github.com/square/finch/stats"
)

// Server coordinates instances: the local and any remotes. Server implements
// Compute so server.Server (the Finch core server) can run as a client or server.
type Server struct {
	api  *API   // handles remote compute (rc)
	name string // defaults to "local"
	test bool
	// --
	gds *data.Scope // global data scope
	cfg config.Stage
}

type ack struct {
	name string // "" for local, else remote.name
	err  error
}

func NewServer(name, addr string, test bool) *Server {
	s := &Server{
		name: name,
		test: test,
		gds:  data.NewScope(), // global data
	}
	if addr != "" {
		s.api = NewAPI(finch.WithPort(addr, finch.DEFAULT_SERVER_PORT))
	}
	return s
}

func (s *Server) Run(ctxFinch context.Context, stages []config.Stage) error {
	for _, cfg := range stages {
		// cd dir of config file so relative file paths in config work
		if err := os.Chdir(filepath.Dir(cfg.File)); err != nil {
			return err
		}

		// Boot the stage: prepares everything, connects to MySQL, but doesn't
		// not execute any queries
		if err := s.run(ctxFinch, cfg); err != nil {
			return err
		}

		if ctxFinch.Err() != nil {
			finch.Debug("finch terminated")
			return nil
		}
	}
	return nil
}

// Run runs all the stages on all the instances (local and remote).
func (s *Server) run(ctxFinch context.Context, cfg config.Stage) error {
	var err error
	stageName := cfg.Name

	nInstances := finch.Uint(cfg.Compute.Instances)
	nRemotes := nInstances - 1 // -1 for local unless..
	if cfg.Compute.DisableLocal {
		nRemotes += 1 // no local, so all instances are remote
	}
	if nRemotes == 0 {
		fmt.Printf("#\n# %s\n#\n", stageName)
	} else {
		cfg.Id = xid.New().String() // unique stage ID for remotes
		fmt.Printf("#\n# %s (%s)\n#\n", stageName, cfg.Id)
	}

	m := &stageMeta{
		Mutex:    &sync.Mutex{},
		cfg:      cfg,
		nRemotes: nRemotes,
		bootChan: make(chan ack, nInstances),
		runChan:  make(chan struct{}),
		doneChan: make(chan ack, nInstances),
		clients:  map[string]*client{},
	}

	if !config.True(cfg.Stats.Disable) {
		m.stats, err = stats.NewCollector(cfg.Stats, s.name, nInstances)
		if err != nil {
			return err
		}
	}

	s.gds.Reset() // keep data from globally-scoped generators; delete the rest

	// Create and boot local instance first because if this doesn't work,
	// then remotes shouldn't work either because they all boot with the
	// exact same config.
	var local *stage.Stage
	if !cfg.Compute.DisableLocal {
		local = stage.New(cfg, s.gds, m.stats)
		if err := local.Prepare(ctxFinch); err != nil {
			return err
		}
		m.bootChan <- ack{name: s.name} // must ack local, too
	}

	// Set stage in API to trigger remote instances to boot
	if s.api != nil && nRemotes > 0 {
		if err := s.api.Stage(m); err != nil {
			return err
		}
	}

	// Wait for the required number instances to boot. If running only local,
	// this will be instant because local already booted and acked above.
	// But with remotes, this might take a few milliseconds over the network.
	if nInstances > 1 {
		log.Printf("Waiting for %d instances to boot...", nInstances)
	}
	booted := uint(0)
	for booted < nInstances {
		select {
		case ack := <-m.bootChan:
			if ack.err != nil {
				log.Printf("Remote %s error on boot: %s", ack.name, ack.err)
				continue
			}
			booted += 1
			if nInstances > 1 {
				log.Printf("%s booted", ack.name)
			}
		case <-ctxFinch.Done():
			return nil
		}
	}

	// Close stage in API to prevent remotes from joining
	m.Lock()
	m.booted = true
	m.Unlock()

	if s.test {
		return nil
	}

	// ----------------------------------------------------------------------
	// Run stage
	// ----------------------------------------------------------------------

	finch.Debug("run %s", stageName)
	close(m.runChan) // signal remotes to run

	if local != nil { // start local instance
		go func() {
			local.Run(ctxFinch)
			m.doneChan <- ack{name: s.name}
		}()
	}

	// Wait for instances to finish running
	running := booted
	for running > 0 {
		select {
		case ack := <-m.doneChan:
			running -= 1
			if ack.err != nil {
				log.Printf("%s error running stage %s: %s", ack.name, stageName, ack.err)
			}
			if nInstances > 1 {
				log.Printf("%s completed stage %s", ack.name, stageName)
				if running > 0 {
					log.Printf("%d/%d instances running", running, nInstances)
				}
			}
		case <-ctxFinch.Done():
			// Signal remote instances to stop early and (maybe) send finals stats
			if s.api != nil {
				s.api.Stage(nil)
			}
		}
	}

	return nil
}
