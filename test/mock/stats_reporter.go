package mock

import (
	"github.com/square/finch/stats"
)

type StatsReporter struct {
	ReportFunc func([]stats.Instance)
	StopFunc   func()
}

func (r StatsReporter) Make(name string, opts map[string]string) (stats.Reporter, error) {
	return r, nil
}

func (r StatsReporter) Report(from []stats.Instance) {
	if r.ReportFunc != nil {
		r.ReportFunc(from)
	}
}

func (r StatsReporter) Stop() {
	if r.StopFunc != nil {
		r.StopFunc()
	}
}
