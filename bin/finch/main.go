// Copyright 2023 Block, Inc.

package main

import (
	"log"

	"github.com/square/finch/boot"
)

func main() {
	if err := boot.Up(boot.Env{}); err != nil {
		log.Fatal(err)
	}
}
