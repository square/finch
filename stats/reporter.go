// Copyright 2022 Block, Inc.

package stats

import (
	"fmt"
	"sync"

	"github.com/square/finch"
	"github.com/square/finch/config"
)

type Reporter interface {
	Report([]Stats)
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
