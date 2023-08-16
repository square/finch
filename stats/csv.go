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
	file *os.File
	p    []float64
}

var _ Reporter = &CSV{}

func NewCSV(opts map[string]string) (*CSV, error) {
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
		return nil, err
	}
	log.Printf("CSV file: %s\n", f.Name())

	sP, nP, err := ParsePercentiles(opts["percentiles"])
	if err != nil {
		return nil, err
	}

	// @todo ensure at least 1 P enforced somewhere

	fmt.Fprintf(f, Header,
		strings.Join(sP, ","),                   // P total
		strings.Join(withPrefix(sP, "r_"), ","), // read
		strings.Join(withPrefix(sP, "w_"), ","), // write
		strings.Join(withPrefix(sP, "c_"), ","), // commit
	)
	fmt.Fprintln(f)

	r := &CSV{
		file: f,
		p:    nP,
	}
	return r, nil
}

func (r *CSV) Report(from []Instance) {
	total := NewStats()
	total.Copy(from[0].Total)
	clients := from[0].Clients
	for i := range from[1:] {
		total.Combine(from[1+i].Total)
		clients += from[1+i].Clients
	}
	compute := from[0].Hostname
	if len(from) > 1 {
		compute = fmt.Sprintf("%d combined", len(from))
	}

	var errorCount uint64
	for _, v := range total.Errors {
		errorCount += v
	}

	// Fill in the line with values except the P percentile values, which is done below
	// because there's a variable number of them
	line := fmt.Sprintf(Fmt,
		from[0].Interval,
		from[0].Seconds, // duration (of interval)
		from[0].Runtime,
		clients,

		// TOTAL
		int64(float64(total.N[TOTAL])/from[0].Seconds), // QPS
		total.Min[TOTAL],
		// P
		total.Max[TOTAL],

		// READ
		int64(float64(total.N[READ])/from[0].Seconds),
		total.Min[READ],
		// P
		total.Max[READ],

		// WRITE
		int64(float64(total.N[WRITE])/from[0].Seconds),
		total.Min[WRITE],
		// P
		total.Max[WRITE],

		// COMMIT
		int64(float64(total.N[COMMIT])/from[0].Seconds), // TPS
		total.Min[COMMIT],
		// P
		total.Max[COMMIT],

		errorCount,

		// Compute (hostname)
		compute,
	)

	// Replace P in Fmt with the CSV percentile values
	line = strings.Replace(line, "P", intsToString(total.Percentiles(TOTAL, r.p), ",", false), 1)
	line = strings.Replace(line, "P", intsToString(total.Percentiles(READ, r.p), ",", false), 1)
	line = strings.Replace(line, "P", intsToString(total.Percentiles(WRITE, r.p), ",", false), 1)
	line = strings.Replace(line, "P", intsToString(total.Percentiles(COMMIT, r.p), ",", false), 1)

	fmt.Fprintln(r.file, line)
}

func (r *CSV) Stop() {
	r.file.Close()
}

func (r *CSV) File() string {
	return r.file.Name()
}
