// Copyright 2022 Block, Inc.

package stats

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	h "github.com/dustin/go-humanize"
)

// Stdout is a Reporter that prints stats to STDOUT. This is the default when
// config.stats is not set.
type Stdout struct {
	p          []float64
	perCompute bool
	w          *tabwriter.Writer
	header     string
	total      *Stats
}

var _ Reporter = &Stdout{}

func NewStdout(opts map[string]string) (*Stdout, error) {
	sP, nP, err := ParsePercentiles(opts["percentiles"])
	if err != nil {
		return nil, err
	}
	// Default header but s/,/\t/g
	header := fmt.Sprintf(Header,
		strings.Join(sP, ","),                   // P total
		strings.Join(withPrefix(sP, "r_"), ","), // read
		strings.Join(withPrefix(sP, "w_"), ","), // write
		strings.Join(withPrefix(sP, "c_"), ","), // commit
	)
	header = strings.ReplaceAll(header, ",", "\t")
	r := &Stdout{
		p:          nP,
		perCompute: opts["per-compute"] == "yes",
		w:          tabwriter.NewWriter(os.Stdout, 1, 0, 1, ' ', tabwriter.AlignRight|tabwriter.Debug),
		header:     header,
	}
	if !r.perCompute {
		r.total = NewStats()
	}
	return r, nil
}

func (r *Stdout) Report(from []Instance) {
	fmt.Fprintln(r.w, r.header)

	// Stats per compute, if enabled
	if r.perCompute {
		for i := range from {
			r.print(from[i].Total, from[i], from[i].Hostname)
		}
	} else {
		from[0].Total.Copy(r.total)
		for i := range from[1:] {
			r.total.Combine(from[1+i].Total)
		}
		compute := from[0].Hostname
		if len(from) > 1 {
			compute = fmt.Sprintf("(%d combined)", len(from))
		}
		r.print(r.total, from[0], compute)
	}
	r.w.Flush()
	fmt.Println()
}

func (r *Stdout) print(s *Stats, in Instance, hostname string) {
	line := fmt.Sprintf("%d\t%1.f\t%d\t%d\t%s\t%s\tP\t%s\t%s\t%s\tP\t%s\t%s\t%s\tP\t%s\t%s\t%s\tP\t%s\t%s\n",
		in.Interval,
		in.Seconds, // duration (of interval)
		in.Runtime,
		in.Clients,

		// TOTAL
		h.Comma(int64(float64(s.N[TOTAL])/in.Seconds)), // QPS
		h.Comma(s.Min[TOTAL]),
		// P
		h.Comma(s.Max[TOTAL]),

		// READ
		h.Comma(int64(float64(s.N[READ])/in.Seconds)),
		h.Comma(s.Min[READ]),
		// P
		h.Comma(s.Max[READ]),

		// WRITE
		h.Comma(int64(float64(s.N[WRITE])/in.Seconds)),
		h.Comma(s.Min[WRITE]),
		// P
		h.Comma(s.Max[WRITE]),

		// COMMIT
		h.Comma(int64(float64(s.N[COMMIT])/in.Seconds)), // TPS
		h.Comma(s.Min[COMMIT]),
		// P
		h.Comma(s.Max[COMMIT]),

		// Compute (hostname)
		hostname,
	)

	// Replace P in Fmt with the CSV percentile values
	line = strings.Replace(line, "P", intsToString(s.Percentiles(TOTAL, r.p), "\\t", true), 1)
	line = strings.Replace(line, "P", intsToString(s.Percentiles(READ, r.p), "\\t", true), 1)
	line = strings.Replace(line, "P", intsToString(s.Percentiles(WRITE, r.p), "\\t", true), 1)
	line = strings.Replace(line, "P", intsToString(s.Percentiles(COMMIT, r.p), "\\t", true), 1)

	fmt.Fprintf(r.w, line)
}

func (r *Stdout) Stop() {}
