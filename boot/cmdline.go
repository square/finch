// Copyright 2023 Block, Inc.

package boot

import (
	"fmt"

	"github.com/alexflint/go-arg"

	"github.com/square/finch"
)

// Options represents the command line options
type Options struct {
	Bind    string `arg:"env:FINCH_BIND" default:":33075"`
	Debug   bool   `arg:"env:FINCH_DEBUG"`
	DSN     string `arg:"env:FINCH_DSN"`
	Help    bool
	Params  []string `arg:"-p,--param,separate"`
	Server  string   `arg:"env:FINCH_SERVER"`
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
		"  --bind ADDR[:PORT]    Listen for clients on ADDR, or \"-\" to disable server API (default: :%s)\n"+
		"  --debug               Print debug output to stderr\n"+
		"  --dsn                 MySQL DSN (overrides stage files)\n"+
		"  --help                Print help and exit\n"+
		"  --param (-p) KEY=VAL  Set param key=value (override stage files)\n"+
		"  --server ADDR[:PORT]  Connect to server at ADDR\n"+
		"  --test                Validate stages, test connections, and exit\n"+
		"  --version             Print version and exit\n"+
		"\n"+
		"finch %s\n",
		finch.DEFAULT_SERVER_PORT,
		finch.VERSION,
	)
}
