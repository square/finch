// Copyright 2022 Block, Inc.

package stats

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/square/finch"
	"github.com/square/finch/proto"
)

// Server is a Reporter that sends stats to a remote compute instance (--server).
// When running as a client, Finch uses and configures this reporter automatically
// in compute/Remote.Boot.
type Server struct {
	server    string // for logging
	client    *proto.Client
	statsChan chan Stats
	stopChan  chan struct{}
	doneChan  chan struct{}
}

var _ Reporter = Server{}

func NewServer(opts map[string]string) (Server, error) {
	r := Server{
		server:    opts["server"], // for logging
		client:    proto.NewClient(opts["client"], opts["server"]),
		statsChan: make(chan Stats, 3),

		stopChan: make(chan struct{}),
		doneChan: make(chan struct{}),
	}
	go r.report()
	return r, nil
}

func (r Server) Report(stats []Stats) {
	if len(stats) != 1 {
		panic(fmt.Sprintf("stats/Server.Report passed %d stats, expected 1", len(stats)))
	}

	// The Collector calls this func at the configured frequency
	// (config.stats.freq), and then we queue the stats via a channel
	// to the report() goroutine that sends them. Async sending with
	// the chan/goroutine allows us to handle intermittent network issues,
	// i.e. don't block in this func, else it'll block Collector and
	// mess up the timing of collecting the stats.
	select {
	case r.statsChan <- stats[0]:
	default:
		log.Printf("Stats dropped because remote is not responding: %+v", stats)
	}
}

func (r Server) Stop() {
	close(r.stopChan)
	select {
	case <-time.After(5 * time.Second):
		log.Println("Timeout sending last stats")
	case <-r.doneChan:
		finch.Debug("remote stats done")
	}
}

func (r Server) report() {
	defer close(r.doneChan)
	for {
		select {
		case <-r.stopChan:
			return
		case s := <-r.statsChan:
			if err := r.client.Send(context.Background(), "/stats", s); err != nil {
				log.Printf("Failed to send stats: %s\n%+v\n", err, s)
				continue
			}
			finch.Debug("sent stats to %s", r.server)
		}
	}
}
