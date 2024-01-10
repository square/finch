// Copyright 2024 Block, Inc.

package stats_test

import (
	"testing"
	"time"

	"github.com/go-test/deep"

	"github.com/square/finch/config"
	"github.com/square/finch/stats"
	"github.com/square/finch/test/mock"
)

func TestCollector_1Client(t *testing.T) {
	var gotStats []stats.Instance
	r := mock.StatsReporter{
		ReportFunc: func(from []stats.Instance) {
			gotStats = make([]stats.Instance, len(from))
			copy(gotStats, from)
		},
	}
	stats.Register("mock", r) // needs a unique reporter name

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
	c.Stop(1*time.Second, false)

	if len(gotStats) == 0 {
		t.Fatal("got zero stats, expected 1")
	}

	s1 := stats.NewStats()
	// {READ, WRITE, COMMIT, TOTAL}
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
			Runtime:  5.0,
			Total:    s1,
			Trx:      map[string]*stats.Stats{"t1": s1},
		},
	}

	deep.FloatPrecision = 3 // Seconds and Runtime will be like 5.000000368--close enough

	if diff := deep.Equal(gotStats, expectStats); diff != nil {
		t.Error(diff)
	}
}

func TestCollector_2Clients(t *testing.T) {
	var gotStats []stats.Instance
	r := mock.StatsReporter{
		ReportFunc: func(from []stats.Instance) {
			gotStats = make([]stats.Instance, len(from))
			copy(gotStats, from)
		},
	}
	stats.Register("mock2", r) // needs a unique reporter name

	cfg := config.Stats{
		Report: map[string]map[string]string{
			"mock2": nil,
		},
	}
	c, err := stats.NewCollector(cfg, "local", 1)
	if err != nil {
		t.Fatal(err)
	}

	c1trx1 := stats.NewTrx("t1")
	c.Watch([]*stats.Trx{c1trx1}) // client 1

	c2trx1 := stats.NewTrx("t1")
	c.Watch([]*stats.Trx{c2trx1}) // client 2

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
	c1trx1.Record(stats.READ, 100)
	c1trx1.Record(stats.READ, 111)

	c2trx1.Record(stats.READ, 200)
	c2trx1.Record(stats.READ, 222)

	c.Stop(1*time.Second, false)

	if len(gotStats) == 0 {
		t.Fatal("got zero stats, expected 1")
	}

	s1 := stats.NewStats()
	// {READ, WRITE, COMMIT, TOTAL}
	s1.N = []uint64{4, 0, 0, 4}
	s1.Min = []int64{100, 0, 0, 100}
	s1.Max = []int64{222, 0, 0, 222}
	// 50 [95.499259, 100.000000)
	// 53 [109.647820, 114.815362)
	// 66 [199.526231, 208.929613)
	// 67 [208.929613, 218.776162)
	// 68 [218.776162, 229.086765)
	s1.Buckets[stats.READ][50] = 1
	s1.Buckets[stats.READ][53] = 1
	s1.Buckets[stats.READ][66] = 1
	s1.Buckets[stats.READ][68] = 1

	s1.Buckets[stats.TOTAL][50] = 1
	s1.Buckets[stats.TOTAL][53] = 1
	s1.Buckets[stats.TOTAL][66] = 1
	s1.Buckets[stats.TOTAL][68] = 1

	expectStats := []stats.Instance{
		{
			Hostname: "local",
			Clients:  2,
			Interval: 1,
			Seconds:  5.0,
			Runtime:  5.0,
			Total:    s1,
			Trx:      map[string]*stats.Stats{"t1": s1},
		},
	}

	deep.FloatPrecision = 3 // Seconds and Runtime will be like 5.000000368--close enough

	if diff := deep.Equal(gotStats, expectStats); diff != nil {
		t.Error(diff)
	}
}

func TestCollector_Combine(t *testing.T) {
	s1 := stats.NewStats()
	// {READ, WRITE, COMMIT, TOTAL}
	s1.N = []uint64{4, 0, 0, 4}
	s1.Min = []int64{100, 0, 0, 100}
	s1.Max = []int64{222, 0, 0, 222}
	s1.Buckets[stats.READ][50] = 1
	s1.Buckets[stats.READ][53] = 1
	s1.Buckets[stats.READ][66] = 1
	s1.Buckets[stats.READ][68] = 1
	s1.Buckets[stats.TOTAL][50] = 1
	s1.Buckets[stats.TOTAL][53] = 1
	s1.Buckets[stats.TOTAL][66] = 1
	s1.Buckets[stats.TOTAL][68] = 1
	in1 := stats.Instance{
		Hostname: "local",
		Clients:  1,
		Interval: 1,
		Seconds:  5.0,
		Runtime:  5.0,
		Total:    s1,
		Trx:      map[string]*stats.Stats{"t1": s1},
	}

	all := stats.NewInstance("local")
	all.Combine([]stats.Instance{in1})
	all.Hostname = "" // don't care about this; care about the numbers
	if diff := deep.Equal(all.Total, s1); diff != nil {
		t.Error(diff)
	}

	s2 := stats.NewStats()
	// {READ, WRITE, COMMIT, TOTAL}
	s2.N = []uint64{1, 0, 0, 1}
	s2.Min = []int64{210, 0, 0, 210}
	s2.Max = []int64{210, 0, 0, 210}
	s2.Buckets[stats.READ][67] = 1
	s2.Buckets[stats.TOTAL][67] = 1
	in2 := stats.Instance{
		Hostname: "local",
		Clients:  1,
		Interval: 1,
		Seconds:  5.0,
		Runtime:  5.0,
		Total:    s2,
		Trx:      map[string]*stats.Stats{"t1": s2},
	}

	all.Combine([]stats.Instance{in1, in2})

	expect := stats.NewStats()
	expect.N = []uint64{5, 0, 0, 5}
	expect.Min = []int64{100, 0, 0, 100}
	expect.Max = []int64{222, 0, 0, 222}
	expect.Buckets[stats.READ][50] = 1
	expect.Buckets[stats.READ][53] = 1
	expect.Buckets[stats.READ][66] = 1
	expect.Buckets[stats.READ][67] = 1
	expect.Buckets[stats.READ][68] = 1
	expect.Buckets[stats.TOTAL][50] = 1
	expect.Buckets[stats.TOTAL][53] = 1
	expect.Buckets[stats.TOTAL][66] = 1
	expect.Buckets[stats.TOTAL][67] = 1
	expect.Buckets[stats.TOTAL][68] = 1

	if diff := deep.Equal(all.Total, expect); diff != nil {
		t.Error(diff)
	}
}
