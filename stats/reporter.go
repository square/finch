// Copyright 2023 Block, Inc.

package stats

import (
	"fmt"
	"strconv"
	"strings"
	"sync"

	h "github.com/dustin/go-humanize"

	"github.com/square/finch"
	"github.com/square/finch/config"
)

var Header = "interval,duration,runtime,clients,QPS,min,%s,max,r_QPS,r_min,%s,r_max,w_QPS,w_min,%s,w_max,TPS,c_min,%s,c_max,errors,compute"
var Fmt = "%d,%.1f,%.1f,%d,%d,%d,P,%d,%d,%d,P,%d,%d,%d,P,%d,%d,%d,P,%d,%d,%s"

var DefaultPercentiles = []float64{99.9}
var DefaultPercentileNames = []string{"P999"}

type Reporter interface {
	Report(from []Instance)
	Stop()
}

type ReporterFactory interface {
	Make(name string, opts map[string]string) (Reporter, error)
}

func MakeReporters(cfg config.Stats) ([]Reporter, error) {
	all := []Reporter{}
	for name, opts := range cfg.Report {
		finch.Debug("make %s: %+v", name, opts)
		f, ok := r.factory[name]
		if !ok {
			return nil, fmt.Errorf("reporter %s not registered", name)
		}
		r, err := f.Make(name, opts)
		if err != nil {
			return nil, err
		}
		all = append(all, r)
	}
	return all, nil
}

func Register(name string, f ReporterFactory) error {
	r.Lock()
	defer r.Unlock()
	_, ok := r.factory[name]
	if ok {
		return fmt.Errorf("reporter %s already registered", name)
	}
	r.factory[name] = f
	finch.Debug("register reporter %s", name)
	return nil
}

// --------------------------------------------------------------------------

func init() {
	Register("stdout", f)
	Register("server", f)
	Register("csv", f)
}

type repo struct {
	*sync.Mutex
	factory map[string]ReporterFactory
}

var r = &repo{
	Mutex:   &sync.Mutex{},
	factory: map[string]ReporterFactory{},
}

type factory struct{}

var f = factory{}

func (f factory) Make(name string, opts map[string]string) (Reporter, error) {
	switch name {
	case "stdout":
		return NewStdout(opts)
	case "server":
		return NewServer(opts)
	case "csv":
		return NewCSV(opts)
	}
	return nil, fmt.Errorf("reporter %s not registered", name)
}

// --------------------------------------------------------------------------

func ParsePercentiles(pCSV string) ([]string, []float64, error) {
	if strings.TrimSpace(pCSV) == "" {
		return DefaultPercentileNames, DefaultPercentiles, nil
	}
	all := strings.Split(pCSV, ",")
	if len(all) == 0 {
		return DefaultPercentileNames, DefaultPercentiles, nil
	}
	s := []string{}  // name "P99.9"
	p := []float64{} // value 99.9
	for _, raw := range all {
		pStr := strings.TrimLeft(strings.TrimSpace(raw), "Pp") // p99 -> 99
		f, err := strconv.ParseFloat(pStr, 64)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid percentile: %s: %s", pStr, err)
		}
		if f < 0.0 || f > 100.0 {
			return nil, nil, fmt.Errorf("percentile out of range: %s (%f): must be bretween 0 and 100", pStr, f)
		}
		s = append(s, "P"+pStr) // 99 -> P99 (string)
		p = append(p, f)        // 99.0 (float)
	}
	return s, p, nil
}

// intsToString returns []int{1,2,3} as "1,2,3" to replace P in Fmt.
func intsToString(n []uint64, sep string, prettyPrint bool) string {
	if len(n) == 0 {
		return ""
	}
	var s string
	if prettyPrint {
		s = fmt.Sprintf("%s", h.Comma(int64(n[0])))
	} else {
		s = fmt.Sprintf("%d", n[0])
	}
	for _, v := range n[1:] {
		if prettyPrint {
			s += fmt.Sprintf("%s%s", sep, h.Comma(int64(v)))
		} else {
			s += fmt.Sprintf("%s%d", sep, v)
		}
	}
	return s
}

// withPrefix returns a copy of s with each prefixed with p.
func withPrefix(s []string, p string) []string {
	c := make([]string, len(s))
	for i := range s {
		c[i] = p + s[i]
	}
	return c
}
