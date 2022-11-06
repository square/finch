// Copyright 2022 Block, Inc.

package stats

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/dustin/go-humanize"
)

// Stdout is a Reporter that prints stats to STDOUT. This is the default when
// config.stats is not set.
type Stdout struct {
	p          []float64
	perCompute bool
	w          *tabwriter.Writer
	header     string
}

var _ Reporter = &Stdout{}

func NewStdout(opts map[string]string) (*Stdout, error) {
	if v, ok := opts["percentiles"]; !ok || v == "" {
		opts["percentiles"] = "95,99,99.9"
	}
	s, p, err := ParsePercentiles(opts["percentiles"])
	if err != nil {
		return nil, err
	}
	header := "interval\tevents\tseconds\tQPS\tmin\t"
	for _, ps := range s {
		header += ps + "\t"
	}
	header += "max\truntime\tcompute"
	r := &Stdout{
		p:          p,
		perCompute: opts["per-compute"] == "yes",
		w:          tabwriter.NewWriter(os.Stdout, 1, 0, 1, ' ', tabwriter.AlignRight|tabwriter.Debug),
		header:     header,
	}
	return r, nil
}

func (r *Stdout) Report(stats []Stats) {
	if len(stats) == 0 {
		return
	}

	fmt.Fprintln(r.w, r.header)

	// Stats per compute, if enabled
	if r.perCompute {
		for _, s := range stats {
			r.print(s)
		}
	} else {
		total := stats[0]
		for i := range stats[1:] {
			total.Combine(stats[1+i])
		}
		if len(stats) > 1 {
			total.Compute = fmt.Sprintf("(%d combined)", len(stats))
		}
		r.print(total)
	}
	r.w.Flush()
	fmt.Println()
}

func (r *Stdout) print(s Stats) {
	fmt.Fprintf(r.w, "%d\t%d\t%.1f\t%s\t%s\t",
		s.Interval, s.N, s.Seconds, humanize.Comma(int64(float64(s.N)/s.Seconds)), humanize.Comma(s.Min))
	for _, p := range s.Percentiles(r.p) {
		fmt.Fprintf(r.w, "%s\t", humanize.Comma(int64(p)))
	}
	fmt.Fprintf(r.w, "%s\t%d\t%s\n", humanize.Comma(s.Max), s.Runtime, s.Compute)
}

func (r *Stdout) Stop() {}
