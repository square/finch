// Copyright 2022 Block, Inc.

package server

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"

	"github.com/square/finch"
	"github.com/square/finch/compute"
	"github.com/square/finch/config"
)

func init() {
	log.SetFlags(log.Ldate | log.Lmicroseconds | log.Lshortfile)
}

// Env is the startup environment: command line args and environment variables.
// This is mostly used for testing to override the defaults.
type Env struct {
	Args []string
	Env  []string
}

func (e Env) Empty() bool {
	return len(e.Args) == 0 && len(e.Env) == 0
}

var portRe = regexp.MustCompile(`:\d+$`)

type Server struct {
	cmdline CommandLine
	comp    compute.Coordinator
	ctx     context.Context
	cancel  context.CancelFunc
}

func (s *Server) Boot(env Env) error {
	if env.Empty() {
		env = Env{
			Args: os.Args,
			Env:  os.Environ(),
		}
	}

	// Parse command line
	var err error
	s.cmdline, err = ParseCommandLine(env.Args)
	if err != nil {
		return err
	}

	// Set global debug var first because all code calls finch.Debug
	finch.Debugging = s.cmdline.Options.Debug
	finch.Debug("finch %s %+v", finch.VERSION, s.cmdline)

	// Return early (don't boot/run) --help, --verison, and --print-domains
	if s.cmdline.Options.Help {
		printHelp()
		os.Exit(0)
	}
	if s.cmdline.Options.Version {
		fmt.Println("finch", finch.VERSION)
		os.Exit(0)
	}

	// Load config file and validate
	var cfg config.File
	var configFile string
	if s.cmdline.Options.Server == "" {
		// Config file required
		if len(s.cmdline.Args) == 1 {
			log.Fatal("No config file specified")
		}
		configFile = s.cmdline.Args[1]
	} else {
		// If --server is specified, then the config file is optional
		if len(s.cmdline.Args) > 1 {
			configFile = s.cmdline.Args[1]
		}
	}
	if configFile != "" {
		cfg, err = config.Load(configFile)
		if err != nil {
			return err
		}
		// cd dir of config file so relative file paths in config work
		os.Chdir(filepath.Dir(configFile))
	}

	// --server override config.compute.server
	if s.cmdline.Options.Server != "" {
		cfg.Compute.Server = s.cmdline.Options.Server
	}
	// Append :port to server addr if not set
	if cfg.Compute.Server != "" && !portRe.MatchString(cfg.Compute.Server) {
		cfg.Compute.Server = cfg.Compute.Server + config.DEFAULT_BIND
	}

	if err := cfg.Validate(); err != nil {
		log.Fatalf("Invalid config (%s): %s\n", configFile, err)
	}
	finch.Debug("config: %+v", cfg)

	// Create compute: local or remote coordinator
	if cfg.Compute.Server == "" {
		finch.Debug("server mode")
		s.comp = compute.NewCoordinator(cfg)
	} else {
		// @todo: add ":33075" if needed
		finch.Debug("client mode: %s -> %s", cfg.Compute.Name, cfg.Compute.Server)
		s.comp = compute.NewRemote(cfg.Compute.Name, cfg.Compute.Server)
	}

	// Server context that cancels on CTRL-C
	s.ctx, s.cancel = context.WithCancel(context.Background())
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)
		<-c
		log.Println("\nCaught CTRL-C")
		s.cancel()
	}()

	return s.comp.Boot(s.ctx, cfg)
}

func (s *Server) Run() error {
	if !s.cmdline.Options.Run {
		return nil
	}
	return s.comp.Run(s.ctx)
}
