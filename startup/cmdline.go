// Copyright 2023 Block, Inc.

package startup

import (
	"fmt"

	"github.com/alexflint/go-arg"

	"github.com/square/finch"
)

// Options represents the command line options
type Options struct {
	Help    bool
	Debug   bool   `arg:"env:FINCH_DEBUG"`
	Run     bool   `arg:"env:FINCH_RUN" default:"true"`
	Server  string `arg:"env:FINCH_SERVER"`
	Version bool
}

type CommandLine struct {
	Options
	Args []string `arg:"positional"`
}

func ParseCommandLine(args []string) (CommandLine, error) {
	var c CommandLine
	p, err := arg.NewParser(arg.Config{Program: "finch"}, &c)
	if err != nil {
		return c, err
	}
	if err := p.Parse(args); err != nil {
		switch err {
		case arg.ErrHelp:
			c.Help = true
		case arg.ErrVersion:
			c.Version = true
		default:
			return c, fmt.Errorf("Error parsing command line: %s\n", err)
		}
	}
	return c, nil
}

func printHelp() {
	fmt.Printf("Usage:\n"+
		"  finch [options] CONFIG\n\n"+
		"Options:\n"+
		"  --debug        Print debug output to stderr\n"+
		"  --help         Print help and exit\n"+
		"  --run          Run stages; if false, boot but don't run\n"+
		"  --server ADDR  Run as remote compute controlled by server\n"+
		"  --version      Print version and exit\n"+
		"\n"+
		"finch %s\n",
		finch.VERSION,
	)
}
