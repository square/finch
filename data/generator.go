// Copyright 2024 Block, Inc.

package data

import (
	"fmt"
	"math/rand"
	"strconv"
	"time"

	"github.com/square/finch"
)

type ValueFunc func(RunCount) []interface{}

// Generator generates data values for a data key (@d).
type Generator interface {
	Format() (uint, string)
	Copy() Generator
	Values(RunCount) []interface{}
	Scan(any interface{}) error
	Name() string
}

func init() {
	rand.Seed(time.Now().UnixNano())
	/*
		Generator names here must match factory.Make switch cases below
	*/
	// Integer
	Register("int", f)
	Register("int-gaps", f)
	Register("int-range", f)
	Register("int-range-seq", f)
	Register("auto-inc", f)
	// String
	Register("str-fill-az", f)
	// ID
	Register("xid", f)
	Register("client-id", f)
	// Column
	Register("column", f)
}

// Factory makes data generators from day keys (@d).
type Factory interface {
	Make(name, dataKey string, params map[string]string) (Generator, error)
}

type factory struct{}

var f = &factory{}

func (f factory) Make(name, dataKey string, params map[string]string) (Generator, error) {
	finch.Debug("Make %s %s", name, dataKey)
	var g Generator
	var err error
	switch name {
	// Integer
	case "int":
		g, err = NewInt(params)
	case "int-gaps":
		g, err = NewIntGaps(params)
	case "int-range":
		g, err = NewIntRange(params)
	case "int-range-seq":
		g, err = NewIntRangeSeq(params)
	case "auto-inc":
		g, err = NewAutoInc(params)
	// String
	case "str-fill-az":
		g, err = NewStrFillAz(params)
	// ID
	case "xid":
		g = NewXid()
	case "client-id":
		g, err = NewClientId(params)
	// Column
	case "column":
		g = NewColumn(params)
	default:
		err = fmt.Errorf("built-in data factory cannot make %s data generator", name)
	}
	if err != nil {
		return nil, err
	}
	return g, nil
}

var r = &repo{
	f: map[string]Factory{},
}

type repo struct {
	f map[string]Factory
}

func Register(name string, f Factory) error {
	_, have := r.f[name]
	if have {
		return fmt.Errorf("data.Generator %s already registered", name)
	}
	r.f[name] = f
	return nil
}

// Make makes a data generator by name with the given generator-specific params.
func Make(name, dataKey string, params map[string]string) (Generator, error) {
	f, have := r.f[name]
	if !have {
		return nil, fmt.Errorf("data.Generator %s not registered", name)
	}
	return f.Make(name, dataKey, params)
}

func int64From(params map[string]string, key string, n *int64, required bool) error {
	s, ok := params[key]
	if !ok {
		if required {
			return fmt.Errorf("%s required", key)
		}
		return nil
	}
	i, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid %s=%s: %s", key, s, err)
	}
	*n = i
	return nil
}
