// Copyright 2022 Block, Inc.

package main

import (
	"log"
	"runtime"

	"github.com/square/finch/server"
)

func main() {
	runtime.GOMAXPROCS(16)
	s := &server.Server{}
	if err := s.Boot(server.Env{}); err != nil {
		log.Fatal(err)
	}
	if err := s.Run(); err != nil {
		log.Fatal(err)
	}
}
