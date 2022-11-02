// Copyright 2022 Block, Inc.

package stats

import (
	"fmt"
)

// Stdout is a Reporter that prints stats to STDOUT. This is the default when
// config.stats is not set.
type Stdout struct {
	p            []float64
	perCompute   bool
	disableTotal bool
}

var _ Reporter = Stdout{}

func NewStdout(opts map[string]string) (Stdout, error) {
	_, p, err := ParsePercentiles(opts["percentiles"])
	if err != nil {
		return Stdout{}, err
	}
	r := Stdout{
		p:          p,
		perCompute: opts["per-compute"] == "yes",
	}
	return r, nil
}

func (r Stdout) Report(stats []Stats) {
	// Stats per compute, if enabled
	if r.perCompute || len(stats) == 1 {
		for _, s := range stats {
			fmt.Printf("%3d: %s  %d events in %.1f seconds\n%.1f QPS min=%d max=%d %v\n",
				s.Interval, s.Compute, s.N, s.Seconds,
				float64(s.N)/s.Seconds, s.Min, s.Max, s.Percentiles(r.p))
		}
	}

	if r.disableTotal || len(stats) == 1 {
		return
	}

	total := stats[0]
	for i := range stats[1:] {
		total.Combine(stats[i])
	}
	fmt.Printf("%3d [%d]: %s  %d events in %.1f seconds\n%.1f QPS min=%d max=%d %v\n",
		total.Interval, total.Runtime, "total", total.N, total.Seconds,
		float64(total.N)/total.Seconds, total.Min, total.Max, total.Percentiles(r.p))
}
