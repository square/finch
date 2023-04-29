// Copyright 2023 Block, Inc.

package boot

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/common-nighthawk/go-figure"

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

// Up is called in main.go to boot up and run Finch.
func Up(env Env) error {
	if len(env.Args) == 0 && len(env.Env) == 0 {
		env = Env{
			Args: os.Args,
			Env:  os.Environ(),
		}
	}

	// Parse command line
	cmdline, err := ParseCommandLine(env.Args)
	if err != nil {
		return err
	}

	// Set global debug var first because all code calls finch.Debug
	finch.Debugging = cmdline.Options.Debug
	finch.Debug("finch %s %+v", finch.VERSION, cmdline)

	// Return early (don't boot/run) --help and --verison
	if cmdline.Options.Help {
		printHelp()
		return nil
	}
	if cmdline.Options.Version {
		fmt.Println("finch", finch.VERSION)
		return nil
	}

	// Catch CTRL-C and cancel the main context, which should cause a clean shutdown
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)
		<-c
		log.Println("\nCaught CTRL-C")
		cancel()
	}()

	//  If --client specified, run in client mode connected to a Finch server.
	// In client mode, we don't need a config file because everything is fetched
	// from the server.
	if serverAddr := cmdline.Options.Client; serverAddr != "" {
		clientName, _ := os.Hostname()
		client := compute.NewClient(clientName, finch.WithPort(serverAddr, finch.DEFAULT_SERVER_PORT))
		return client.Run(ctx)
	}

	// ----------------------------------------------------------------------
	// Server mode (default)

	// Load and validate all stage config files specified on the command line
	if len(cmdline.Args) == 1 {
		log.Fatal("No config file specified")
	}
	cfgStages, err := config.Load(cmdline.Args[1:], cmdline.Options.Params)
	if err != nil {
		log.Fatal(err)
	}

	// Boot and run each stage specified on the command line
	server := compute.NewServer("local", cmdline.Options.Server)
	for _, cfg := range cfgStages {
		// cd dir of config file so relative file paths in config work
		if err := os.Chdir(filepath.Dir(cfg.FileName)); err != nil {
			log.Fatal(err)
		}

		// Print stage name as big ASCII text banner to make it easier to see
		// separate stages
		myFigure := figure.NewFigure(cfg.Name, "", true)
		myFigure.Print()

		// Boot the stage: prepares everything, connects to MySQL, but doesn't
		// not execute any queries
		if err := server.Boot(ctx, cfg); err != nil {
			log.Fatal(err)
		}

		// If --test, don't run the stage; just boot the next stage
		if cmdline.Options.Test {
			continue
		}

		// Run the stage. This is where all the traditional benchmark work starts.
		if err := server.Run(ctx); err != nil {
			log.Println(err)
		}
	}

	return nil
}
