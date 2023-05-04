// Copyright 2023 Block, Inc.

package boot

import (
	"fmt"

	"github.com/alexflint/go-arg"

	"github.com/square/finch"
)

// Options represents the command line options
type Options struct {
	Help    bool
	Client  string   `arg:"env:FINCH_CLIENT"`
	Debug   bool     `arg:"env:FINCH_DEBUG"`
	DSN     string   `arg:"env:FINCH_DSN"`
	Params  []string `arg:"-p,--param,separate"`
	Server  string   `arg:"env:FINCH_SERVER" default:"127.0.0.1:33075"`
	Test    bool     `arg:"env:FINCH_TEST"`
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
		"  finch [options] STAGE_1_FILE [STAGE_N_FILE...]\n\n"+
		"Options:\n"+
		"  --client ADDR[:PORT]  Run as client controlled by server at ADDR (default port: %s)\n"+
		"  --debug               Print debug output to stderr\n"+
		"  --dsn                 MySQL DSN (overrides stage files)\n"+
		"  --help                Print help and exit\n"+
		"  --param (-p) KEY=VAL  Set param key=value (override stage files)\n"+
		"  --server ADDR[:PORT]  Run as server listening for clients on ADDR (default port: %s)\n"+
		"  --test                Validate stages, test connections, and exit\n"+
		"  --version             Print version and exit\n"+
		"\n"+
		"finch %s\n",
		finch.DEFAULT_SERVER_PORT,
		finch.DEFAULT_SERVER_PORT,
		finch.VERSION,
	)
}
