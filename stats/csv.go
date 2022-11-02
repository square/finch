// Copyright 2022 Block, Inc.

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

var _ Reporter = CSV{}

func NewCSV(opts map[string]string) (CSV, error) {
	var f *os.File
	var err error
	fileName := opts["file"]
	if fileName == "" {
		// Use a random temp file
		f, err = os.CreateTemp("", fmt.Sprintf("finch-benchmark-%s", strings.ReplaceAll(time.Now().Format(time.Stamp), " ", "_")))
	} else {
		f, err = os.OpenFile(fileName, os.O_APPEND|os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	}
	if err != nil {
		return CSV{}, err
	}
	log.Println("CSV file: %s", f.Name())

	n, p, err := ParsePercentiles(opts["percentiles"])
	if err != nil {
		return CSV{}, err
	}

	fmt.Fprintf(f, "interval,runtime,QPS,min,max")
	if len(p) > 0 {
		fmt.Fprintf(f, ","+strings.Join(n, ","))
	}
	fmt.Fprintln(f)

	r := CSV{
		p:    p,
		file: f,
	}
	return r, nil
}

func (r CSV) Report(stats []Stats) {
	total := stats[0]
	if len(stats) > 1 {
		for i := range stats[1:] {
			total.Combine(stats[i])
		}
	}
	// interval,runtime,QPS,min,max,P...
	fmt.Fprintf(r.file, "%d,%d,%.1f,%d,%d",
		total.Interval,
		total.Runtime,
		float64(total.N)/total.Seconds,
		total.Min,
		total.Max,
	)
	for _, p := range total.Percentiles(r.p) {
		fmt.Fprintf(r.file, ",%d", p)
	}
	fmt.Fprintln(r.file)
}
