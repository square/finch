// Copyright 2023 Block, Inc.

package stats

import (
	"fmt"
	"log"
	"os"
	"strings"
	"time"
)

// CSV is a Reporter that prints stats to STDOUT. This is the default when
// config.stats is not set.
type CSV struct {
	file  *os.File
	p     []float64
	total *Stats
}

var _ Reporter = CSV{}

func NewCSV(opts map[string]string) (CSV, error) {
	var f *os.File
	var err error
	fileName := opts["file"]
	if fileName == "" {
		// Use a random temp file
		f, err = os.CreateTemp("", fmt.Sprintf("finch-benchmark-%s.csv", strings.ReplaceAll(time.Now().Format(time.Stamp), " ", "_")))
	} else {
		f, err = os.OpenFile(fileName, os.O_APPEND|os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	}
	if err != nil {
		return CSV{}, err
	}
	log.Printf("CSV file: %s\n", f.Name())

	sP, nP, err := ParsePercentiles(opts["percentiles"])
	if err != nil {
		return CSV{}, err
	}

	// @todo ensure at least 1 P enforced somewhere

	fmt.Fprintf(f, Header,
		strings.Join(sP, ","),                   // P total
		strings.Join(withPrefix(sP, "r_"), ","), // read
		strings.Join(withPrefix(sP, "w_"), ","), // write
		strings.Join(withPrefix(sP, "c_"), ","), // commit
	)
	fmt.Fprintln(f)

	r := CSV{
		file:  f,
		p:     nP,
		total: NewStats(),
	}
	return r, nil
}

func (r CSV) Report(from []Instance) {
	from[0].Total.Copy(r.total)
	clients := from[0].Clients
	for i := range from[1:] {
		r.total.Combine(from[1+i].Total)
		clients += from[1+1].Clients
	}
	compute := from[0].Hostname
	if len(from) > 1 {
		compute = fmt.Sprintf("%d combined", len(from))
	}

	// Fill in the line with values except the P percentile values, which is done below
	// because there's a variable number of them
	line := fmt.Sprintf(Fmt,
		from[0].Interval,
		from[0].Seconds, // duration (of interval)
		from[0].Runtime,
		clients,

		// TOTAL
		int64(float64(r.total.N[TOTAL])/from[0].Seconds), // QPS
		r.total.Min[TOTAL],
		// P
		r.total.Max[TOTAL],

		// READ
		int64(float64(r.total.N[READ])/from[0].Seconds),
		r.total.Min[READ],
		// P
		r.total.Max[READ],

		// WRITE
		int64(float64(r.total.N[WRITE])/from[0].Seconds),
		r.total.Min[WRITE],
		// P
		r.total.Max[WRITE],

		// COMMIT
		int64(float64(r.total.N[COMMIT])/from[0].Seconds), // TPS
		r.total.Min[COMMIT],
		// P
		r.total.Max[COMMIT],

		// Compute (hostname)
		compute,
	)

	// Replace P in Fmt with the CSV percentile values
	line = strings.Replace(line, "P", intsToString(r.total.Percentiles(TOTAL, r.p), ",", false), 1)
	line = strings.Replace(line, "P", intsToString(r.total.Percentiles(READ, r.p), ",", false), 1)
	line = strings.Replace(line, "P", intsToString(r.total.Percentiles(WRITE, r.p), ",", false), 1)
	line = strings.Replace(line, "P", intsToString(r.total.Percentiles(COMMIT, r.p), ",", false), 1)

	fmt.Fprintln(r.file, line)
}

func (r CSV) Stop() {
	r.file.Close()
}
