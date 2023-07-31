// Copyright 2023 Block, Inc.

package stats_test

import (
	"os"
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

func TestCSV(t *testing.T) {
	r, err := stats.NewCSV(map[string]string{})
	if err != nil {
		t.Fatal(err)
	}

	file := r.File()
	t.Logf("stats file: %s", file)

	s := stats.NewStats()
	s.Record(stats.READ, 110)
	s.Record(stats.READ, 190)
	s.Record(stats.WRITE, 210)
	s.Record(stats.WRITE, 290)
	s.Record(stats.COMMIT, 310)
	s.Record(stats.COMMIT, 390)

	from := []stats.Instance{
		{
			Hostname: "local",
			Clients:  1,
			Interval: 1,
			Seconds:  2.0,
			Runtime:  2.0,
			Total:    s,
			//Trx:      map[string]*stats.Stats{},
		},
	}
	r.Report(from)
	r.Stop()

	got, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	expect := `interval,duration,runtime,clients,QPS,min,P999,max,r_QPS,r_min,r_P999,r_max,w_QPS,w_min,w_P999,w_max,TPS,c_min,c_P999,c_max,errors,compute
1,2.0,2.0,1,3,110,389,390,1,110,185,190,1,210,294,290,1,310,389,390,0,local
`
	if string(got) != expect {
		t.Errorf("got:\n%s\nexpected:\n%s\n", string(got), expect)
	}

	err = os.Remove(file)
	if err != nil {
		t.Error(err)
	}
}
