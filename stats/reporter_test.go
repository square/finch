// Copyright 2023 Block, Inc.

package stats_test

import (
	"testing"

	"github.com/go-test/deep"

	"github.com/square/finch/stats"
)

func TestParsePercentiles(t *testing.T) {
	tests := []struct {
		in string
		s  []string
		p  []float64
	}{
		{"", stats.DefaultPercentileNames, stats.DefaultPercentiles},
		{"95", []string{"P95"}, []float64{95.0}},
		{"95,99", []string{"P95", "P99"}, []float64{95.0, 99.0}},
		{"P99.9", []string{"P99.9"}, []float64{99.9}},
	}
	for _, test := range tests {
		t.Run(test.in, func(t *testing.T) {
			s, p, err := stats.ParsePercentiles(test.in)
			if err != nil {
				t.Error(err)
			}
			if diff := deep.Equal(s, test.s); diff != nil {
				t.Error(diff)
			}
			if diff := deep.Equal(p, test.p); diff != nil {
				t.Error(diff)
			}

		})
	}
}
