// Copyright 2022 Block, Inc.

package stats_test

import (
	"testing"

	"github.com/go-test/deep"

	"github.com/square/finch/stats"
)

func TestBasicStats(t *testing.T) {
	s := stats.NewStats()

	s.Reset()

	s.Record(stats.READ, 200)
	s.Record(stats.READ, 200)
	s.Record(stats.READ, 200)

	s.Record(stats.TOTAL, 100)
	s.Record(stats.TOTAL, 100)

	snapshot := s.Snapshot()
	if snapshot.N[stats.TOTAL] != 5 {
		t.Errorf("got %d events total, expected 5", snapshot.N[stats.TOTAL])
	}

	if snapshot.N[stats.READ] != 3 {
		t.Errorf("got %d reads, expected 3", snapshot.N[stats.READ])
	}

	// @todo finish
}

func TestPecentiles_P9s(t *testing.T) {
	v := [][]int64{
		{125000, 1},  // 125 ms  -- 125892.541179 (205) -- P0.38
		{200000, 10}, // 200 ms  -- 208929.613085 (216) -- P4.20
		{255000, 20}, // 255 ms  -- 251188.643151 (221) -- P11.83
		{289000, 50}, // 289 ms  -- 301995.172040 (224) -- P30.92
		//                       -- 309111              ~~ P50
		{302000, 100}, // 300 ms -- 316227.766017 (225) -- P69.08
		{321000, 70},  // 310 ms -- 331131.121483 (226) -- P95.80
		{450000, 10},  // 450 ms -- 457088.189615 (233) -- P99.62
		{605000, 1},   // 605 ms -- 630957.344480 (240) -- P100.00
		//    = 262
	}

	s := stats.NewStats()
	for i := range v {
		for j := int64(0); j < v[i][1]; j++ {
			s.Record(stats.TOTAL, v[i][0])
		}
	}

	if s.N[stats.TOTAL] != 262 {
		t.Errorf("N = %d, expected 262", s.N)
	}

	p := s.Percentiles(stats.TOTAL, []float64{50, 95, 99, 99.9})
	expect := []uint64{
		309111, // P50
		331131, // P95
		457088, // P99
		616758, // P999
	}
	if diff := deep.Equal(p, expect); diff != nil {
		t.Error(diff)
	}
}

func TestPecentiles_P50(t *testing.T) {
	v := [][]int64{
		{200000, 10}, // 200 ms  -- 208929.613085 (216)
		{255000, 10}, // 255 ms  -- 251188.643151 (221)
		{289000, 10}, // 289 ms  -- 301995.172040 (224)
		//
		{302000, 10}, // 300 ms -- 316227.766017 (225)
		{321000, 10}, // 310 ms -- 331131.121483 (226)
		{450000, 10}, // 450 ms -- 457088.189615 (233)
	}

	s := stats.NewStats()
	for i := range v {
		for j := int64(0); j < v[i][1]; j++ {
			s.Record(stats.TOTAL, v[i][0])
		}
	}

	if s.N[stats.TOTAL] != 60 {
		t.Errorf("N = %d, expected 60", s.N)
	}

	p := s.Percentiles(stats.TOTAL, []float64{50})
	expect := []uint64{301995}
	if diff := deep.Equal(p, expect); diff != nil {
		t.Error(diff)
	}
}
