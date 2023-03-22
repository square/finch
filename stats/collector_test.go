// Copyright 2022 Block, Inc.

package stats_test

import (
	"testing"
	"time"

	"github.com/go-test/deep"

	"github.com/square/finch/config"
	"github.com/square/finch/stats"
	"github.com/square/finch/test/mock"
)

func TestCollector(t *testing.T) {
	var gotStats []stats.Instance
	r := mock.StatsReporter{
		ReportFunc: func(from []stats.Instance) {
			gotStats = make([]stats.Instance, len(from))
			copy(gotStats, from)
		},
	}
	stats.Register("mock", r) // @todo might need to de-register for other tests?

	cfg := config.Stats{
		Report: map[string]map[string]string{
			"mock": nil,
		},
	}
	c, err := stats.NewCollector(cfg, "local", 1)
	if err != nil {
		t.Fatal(err)
	}

	trx1 := stats.NewTrx("t1")
	c.Watch([]*stats.Trx{trx1})

	// Fake time for Now
	ti := 0
	times := []time.Time{
		time.Now().Add(time.Duration(-6) * time.Second),
		// 5s
		time.Now().Add(time.Duration(-1) * time.Second),
	}
	stats.Now = func() time.Time {
		now := times[ti]
		ti += 1
		return now
	}
	defer func() { stats.Now = time.Now }()

	c.Start()
	trx1.Record(stats.READ, 210)
	c.Stop()

	if len(gotStats) == 0 {
		t.Fatal("got zero stats, expected 1")
	}

	s1 := stats.NewStats()
	s1.N = []uint64{1, 0, 0, 1}
	s1.Min = []int64{210, 0, 0, 210}
	s1.Max = []int64{210, 0, 0, 210}
	// bucket 67 [208.929613, 218.776162)
	s1.Buckets[stats.READ][67] = 1
	s1.Buckets[stats.TOTAL][67] = 1

	expectStats := []stats.Instance{
		{
			Hostname: "local",
			Clients:  1,
			Interval: 1,
			Seconds:  5.0,
			Runtime:  5,
			Total:    s1,
			Trx:      map[string]*stats.Stats{"t1": s1},
		},
	}

	// Even using fake times ^, Go returns 5.000000147, but we can safely
	// ignore those 147 nanoseconds--1ms of precision is enough.
	if gotStats[0].Seconds < 5.0 || gotStats[0].Seconds > 5.001 {
		t.Errorf("Stats.Seconds = %f, expected [5.0, 5.001]", gotStats[0].Seconds)
	}
	gotStats[0].Seconds = 5.0 // make even 5.0 so deep.Equal works

	if diff := deep.Equal(gotStats, expectStats); diff != nil {
		t.Error(diff)
	}
}
