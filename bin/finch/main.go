// Copyright 2022 Block, Inc.

package main

import (
	"log"

	"github.com/square/finch/startup"
)

func main() {
	f := &startup.Finch{}

	if err := f.Boot(startup.Env{}); err != nil {
		log.Fatal(err)
	}

	if err := f.Run(); err != nil {
		log.Fatal(err)
	}
}
