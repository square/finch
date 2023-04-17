// Copyright 2023 Block, Inc.

package main

import (
	"log"
	"runtime"

	"github.com/square/finch/startup"
)

func main() {
	runtime.GOMAXPROCS(2 * runtime.NumCPU())

	f := &startup.Finch{}

	if err := f.Boot(startup.Env{}); err != nil {
		log.Fatal(err)
	}

	if err := f.Run(); err != nil {
		log.Fatal(err)
	}
}
