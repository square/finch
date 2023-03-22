// Copyright 2022 Block, Inc.

package main

import (
	"log"

	"github.com/square/finch/server"
)

func main() {
	//runtime.GOMAXPROCS(16)

	s := &server.Server{}
	if err := s.Boot(server.Env{}); err != nil {
		log.Fatal(err)
	}

	/*
		f, err := os.Create("cpu.prof")
		if err != nil {
			log.Fatal(err)
		}
		pprof.StartCPUProfile(f)
	*/

	if err := s.Run(); err != nil {
		log.Fatal(err)
	}
	//pprof.StopCPUProfile()
}
