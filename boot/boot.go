// Copyright 2023 Block, Inc.

package boot

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/square/finch"
	"github.com/square/finch/compute"
	"github.com/square/finch/config"
)

func init() {
	log.SetOutput(os.Stdout)
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

	log.Println(finch.SystemParams)

	// Catch CTRL-C and cancel the main context, which should cause a clean shutdown
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)
		<-c
		log.Println("Caught CTRL-C")
		cancel()
		<-time.After(7500 * time.Millisecond) // 7.5s
		os.Exit(0)
	}()

	//  If --client specified, run in client mode connected to a Finch server.
	// In client mode, we don't need a config file because everything is fetched
	// from the server.
	if serverAddr := cmdline.Options.Server; serverAddr != "" {
		clientName, _ := os.Hostname()
		client := compute.NewClient(clientName, finch.WithPort(serverAddr, finch.DEFAULT_SERVER_PORT))
		return client.Run(ctx)
	}

	// ----------------------------------------------------------------------
	// Server mode (default)

	// Load and validate all stage config files specified on the command line
	if len(cmdline.Args) == 1 {
		log.Fatal("No stage file specified. Run finch --help for usage.")
	}
	stages, err := config.Load(
		cmdline.Args[1:],
		cmdline.Options.Params,
		cmdline.Options.DSN,
	)
	if err != nil {
		log.Fatal(err)
	}

	// Boot and run each stage specified on the command line
	server := compute.NewServer("local", cmdline.Options.Bind, cmdline.Options.Test)
	return server.Run(ctx, stages)
}