// Copyright 2023 Block, Inc.

package compute

import (
	"context"
	"log"

	"github.com/rs/xid"

	"github.com/square/finch"
	"github.com/square/finch/config"
	"github.com/square/finch/data"
	"github.com/square/finch/stats"
)

// Server coordinates instances: the local and any remotes. Server implements
// Compute so server.Server (the Finch core server) can run as a client or server.
type Server struct {
	api  *API   // handles remote compute (rc)
	name string // defaults to "local"
	// --
	gds   *data.Scope // global data scope
	stage *metaStage  // current stage
}

type ack struct {
	name string // "" for local, else remote.name
	err  error
}

func NewServer(name, addr string) *Server {
	s := &Server{
		name: name,
		gds:  data.NewScope(), // global data
	}
	if addr != "" {
		s.api = NewAPI(finch.WithPort(addr, finch.DEFAULT_SERVER_PORT))
	}
	return s
}

// Run runs all the stages on all the instances (local and remote).
func (s *Server) Boot(ctxFinch context.Context, cfg config.Stage) error {
	var err error

	nInstances := finch.Uint(cfg.Compute.Instances)
	log.Printf("Boot %s (%d instances)", cfg.Name, nInstances)

	// Clear previous stage, if any. This happens with multiple stages and --run=false.
	// We'll boot 1st stage (but not run), then clear it and boot 2nd stage, etc. This
	// call might block for a few milliseconds because API has to reset remotes that are
	// waiting to run.
	if s.api != nil && s.stage != nil {
		log.Println("Clear previously booted stage")
		if err := s.api.Stage(nil); err != nil {
			return err
		}
	}

	s.gds.Reset() // keep data from globally-scoped generators; delete the rest

	s.stage = &metaStage{
		cfg:        cfg,
		sid:        xid.New().String(),
		nInstances: nInstances,
		bootChan:   make(chan ack, nInstances),
		runChan:    make(chan struct{}),
		doneChan:   make(chan ack, nInstances),
	}

	if !config.True(cfg.Stats.Disable) {
		s.stage.stats, err = stats.NewCollector(cfg.Stats, s.name, nInstances)
		if err != nil {
			return err
		}
	}

	// Create and boot local instance first because if this doesn't work,
	// then remotes shouldn't work either because they all boot with the
	// exact same config.
	if !cfg.Compute.DisableLocal {
		local := NewLocal(s.name, s.gds, s.stage.stats)
		if err := local.Boot(ctxFinch, cfg); err != nil {
			return err
		}
		s.stage.local = local                 // save for Run
		s.stage.bootChan <- ack{name: s.name} // must ack local, too
	}

	// Set stage in API to trigger remote instances to boot
	if s.api != nil {
		if err := s.api.Stage(s.stage); err != nil {
			return err
		}
	}

	// Wait for the required number instances to boot. If running only local,
	// this will be instant because local already booted and acked above.
	// But with remotes, this might take a few milliseconds over the network.
	booted := uint(0)
	for booted < s.stage.nInstances {
		log.Printf("Have %d compute instances, need %d", booted, nInstances)
		select {
		case ack := <-s.stage.bootChan:
			if ack.err != nil {
				log.Printf("Remote %s error on boot: %s", ack.name, ack.err)
				continue
			}
			booted += 1
			log.Printf("%s booted", ack.name)
		case <-ctxFinch.Done():
			return nil
		}
	}

	return nil
}

func (s *Server) Run(ctxFinch context.Context) error {
	if s.stage == nil {
		panic("Server.state is nil")
	}
	stageName := s.stage.cfg.Name
	finch.Debug("run %s", stageName)

	if s.api != nil {
		defer s.api.Stage(nil) // clear stage when done running
	}

	close(s.stage.runChan) // signal remotes to run

	if s.stage.local != nil { // start local instance
		go func() {
			err := s.stage.local.Run(ctxFinch)
			s.stage.doneChan <- ack{name: s.name, err: err}
		}()
	}

	// Wait for instances to finish running
	done := uint(0)
	for done < s.stage.nInstances {
		log.Printf("%d instances running...", s.stage.nInstances-done)
		select {
		case ack := <-s.stage.doneChan:
			done += 1
			if ack.err != nil {
				log.Printf("%s error running stage %s: %s", ack.name, stageName, ack.err)
			} else {
				log.Printf("%s completed stage %s", ack.name, stageName)
			}
		case <-ctxFinch.Done():
			return nil
		}
	}

	return nil
}

func (s *Server) Shutdown() {
	//s.httpServer.Close()
}
