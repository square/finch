// Copyright 2023 Block, Inc.

package stats

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	h "github.com/dustin/go-humanize"
	"github.com/square/finch"
)

// Stdout is a Reporter that prints stats to STDOUT. This is the default when
// config.stats is not set.
//
//	stats:
//	  report:
//	    stdout:
//	      each-instance: true
//	      combined: true
type Stdout struct {
	p        []float64
	w        *tabwriter.Writer
	header   string
	all      *Instance
	each     bool
	combined bool
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
		p:        nP,
		w:        tabwriter.NewWriter(os.Stdout, 1, 0, 1, ' ', tabwriter.AlignRight|tabwriter.Debug),
		header:   header,
		each:     finch.Bool(opts["each-instance"]),
		combined: finch.Bool(opts["combined"]),
	}
	if r.each == false && r.combined == false {
		r.combined = true
	}
	if r.combined {
		r.all = &Instance{
			Total: NewStats(),
			// We don't use Trx stats yet
		}
	}
	return r, nil
}

func (r *Stdout) Report(from []Instance) {
	fmt.Fprintln(r.w, r.header)
	if r.each {
		for i := range from {
			r.print(&from[i])
		}
	}
	if r.combined && len(from) > 1 {
		r.all.Combine(from)
		r.print(r.all)
	}
	r.w.Flush()
	fmt.Println()
}

func (r *Stdout) print(in *Instance) {
	s := in.Total
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

		in.Hostname,
	)

	// Replace P in Fmt with the CSV percentile values
	line = strings.Replace(line, "P", intsToString(s.Percentiles(TOTAL, r.p), "\\t", true), 1)
	line = strings.Replace(line, "P", intsToString(s.Percentiles(READ, r.p), "\\t", true), 1)
	line = strings.Replace(line, "P", intsToString(s.Percentiles(WRITE, r.p), "\\t", true), 1)
	line = strings.Replace(line, "P", intsToString(s.Percentiles(COMMIT, r.p), "\\t", true), 1)

	fmt.Fprintf(r.w, line)
}

func (r *Stdout) Stop() {}
