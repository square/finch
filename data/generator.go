// Copyright 2023 Block, Inc.

package data

import (
	"fmt"
	"math/rand"
	"strconv"
	"time"

	"github.com/square/finch"
)

func init() {
	rand.Seed(time.Now().UnixNano())

	Register("rand-int", f)
	Register("int-range", f)
	Register("auto-inc", f)
	Register("uint64-counter", f)
	Register("project-int", f)
	Register("str-not-null", f)
	Register("column", f)
	Register("xid", f)
}

// Generator generates data values for a day key (@d).
type Generator interface {
	Format() string
	Copy(finch.RunLevel) Generator
	Values(finch.ExecCount) []interface{}
	Scan(any interface{}) error
	Id() Id
}

type Id struct {
	finch.RunLevel
	Scope   string // finch.SCOPE_*
	Type    string // str-not-null
	DataKey string // @col
	CopyNo  uint   // 1..N
	copied  uint
}

func (id Id) String() string {
	return fmt.Sprintf("%s %s(%d)/%s %s", id.RunLevel, id.DataKey, id.CopyNo, id.Scope, id.Type)
}

func (id *Id) Copy(r finch.RunLevel) Id {
	//   ^ Must take pointer so id.copied is maintained/incremented across all copies
	id.copied++           // next CopyNo for this copy
	cp := *id             // copy previous Id
	cp.RunLevel = r       // set RuneLevel for this copy
	cp.CopyNo = id.copied // set CopyNo for this copy

	// If scope wasn't explicitly configured and generator didn't set its default scope,
	// then set default statement scope to ensure Id.Scope is always set.
	if cp.Scope == "" {
		cp.Scope = finch.SCOPE_STATEMENT
	}
	return cp
}

type Factory interface {
	Make(name, dataName, scope string, params map[string]string) (Generator, error)
}

type factory struct{}

var f = &factory{}

func (f factory) Make(name, dataName, scope string, params map[string]string) (Generator, error) {
	id := Id{
		Scope:   scope,
		Type:    name,
		DataKey: dataName,
	}
	switch name {
	case "rand-int":
		return NewRandInt(id, params)
	case "int-range":
		return NewIntRange(id, params)
	case "int-sequence":
		return NewIntSequence(id, params)
	case "uint64-counter":
		return NewIncUint64(id, params), nil
	case "str-not-null":
		s, ok := params["len"]
		if !ok {
			return nil, fmt.Errorf("str-not-null requires param len")
		}
		n, err := strconv.Atoi(s)
		if err != nil {
			return nil, err
		}
		return NewStrNotNull(id, n), nil
	case "column":
		return NewColumn(id, params), nil
	case "xid":
		return NewXid(id), nil
	case "project-int":
		return NewProjectInt(id, params)
	}
	return nil, fmt.Errorf("built-in data factory cannot make %s data generator", name)
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
func Make(name, dataName, scope string, params map[string]string) (Generator, error) {
	f, have := r.f[name]
	if !have {
		return nil, fmt.Errorf("data.Generator %s not registered", name)
	}
	return f.Make(name, dataName, scope, params)
}

// --------------------------------------------------------------------------

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
