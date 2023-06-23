// Copyright 2023 Block, Inc.

package data

import (
	"fmt"
	"math/rand"
	"strconv"
	"time"

	"github.com/square/finch"
)

// Generator generates data values for a data key (@d).
type Generator interface {
	Format() string
	Copy(finch.RunLevel) Generator
	Values(RunCount) []interface{}
	Scan(any interface{}) error
	Id() Id
}

// Id identifies the scope and copy number of a data key (@d).
type Id struct {
	finch.RunLevel
	Scope   string // finch.SCOPE_*
	Type    string // str-az
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
	// Column
	Register("column", f)
}

// Factory makes data generators from day keys (@d).
type Factory interface {
	Make(name, dataName, scope string, params map[string]string) (Generator, error)
}

type factory struct{}

var f = &factory{}

func (f factory) Make(name, dataName, scope string, params map[string]string) (Generator, error) {
	switch scope {
	case "", finch.SCOPE_VALUE, finch.SCOPE_STATEMENT, finch.SCOPE_TRX, finch.SCOPE_CLIENT:
		// valid
	default:
		return nil, fmt.Errorf("%s: invalid data scope: %s; valid values: %s, %s, %s (default: %s)",
			dataName, scope, finch.SCOPE_STATEMENT, finch.SCOPE_TRX, finch.SCOPE_CLIENT, finch.SCOPE_STATEMENT)
	}
	id := Id{
		Scope:   scope,
		Type:    name,
		DataKey: dataName,
	}
	var g Generator
	var err error
	switch name {
	// Integer
	case "int":
		g, err = NewInt(id, params)
	case "int-gaps":
		g, err = NewIntGaps(id, params)
	case "int-range":
		g, err = NewIntRange(id, params)
	case "int-range-seq":
		g, err = NewIntRangeSeq(id, params)
	case "auto-inc":
		g, err = NewAutoInc(id, params)
	// String
	case "str-fill-az":
		g, err = NewStrFillAz(id, params)
	// ID
	case "xid":
		g = NewXid(id)
	// Column
	case "column":
		g = NewColumn(id, params)
	default:
		err = fmt.Errorf("built-in data factory cannot make %s data generator", name)
	}
	if err != nil {
		return nil, err
	}

	// Special case for INSERT INTO t VALUES (@d), (@d), ..., N: same @d but called N times
	if id.Scope == finch.SCOPE_VALUE {
		return g, nil
	}

	// Wrap real generator in ScopedGenerator that handles scope logic
	return NewScopedGenerator(g), nil
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
