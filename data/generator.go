// Copyright 2022 Block, Inc.

package data

import (
	"fmt"
	"strconv"
)

func init() {
	Register("int-not-null", f)
	Register("int-range", f)
	Register("project-int", f)
	Register("str-not-null", f)
	Register("column", f)
	Register("xid", f)
}

type Generator interface {
	Format() string
	Copy(clientNo int) Generator
	Values() []interface{}
	Scan(any interface{}) error
	Id() Id
	Scope() string
}

type Id struct {
	DataName string // @col
	Type     string // str-not-null
	ClientNo int    // 1
	CopyNo   int    // 1
	copied   int
}

func (id Id) String() string {
	return fmt.Sprintf("%s/%s/%d/%d", id.DataName, id.Type, id.ClientNo, id.CopyNo)
}

func (id *Id) Copy(clientNo int) Id {
	id.copied++
	cp := *id
	cp.ClientNo = clientNo
	cp.CopyNo = id.copied
	return cp
}

type Factory interface {
	Make(name, dataName string, params map[string]string) (Generator, error)
}

type factory struct{}

var f = &factory{}

func (f factory) Make(name, dataName string, params map[string]string) (Generator, error) {
	id := Id{
		Type:     name,
		DataName: dataName,
	}
	switch name {
	case "int-not-null":
		max := 2147483647
		if s, ok := params["max"]; ok {
			n, err := strconv.Atoi(s)
			if err != nil {
				return nil, err
			}
			max = n
		}
		return IntNotNull{Max: max}, nil
	case "int-range":
		return NewIntRange(id, params)
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
		return NewColumn(id, params["col"], params["type"]), nil
	case "xid":
		n := 1
		s, ok := params["rows"]
		if ok {
			var err error
			n, err = strconv.Atoi(s)
			if err != nil {
				return nil, err
			}
		}
		return NewXid(id, n), nil
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
func Make(name, dataName string, params map[string]string) (Generator, error) {
	f, have := r.f[name]
	if !have {
		return nil, fmt.Errorf("data.Generator %s not registered", name)
	}
	return f.Make(name, dataName, params)
}

// --------------------------------------------------------------------------

func int64From(params map[string]string, key string, n *int64, required bool) error {
	s, ok := params[key]
	if !ok {
		if required {
			return fmt.Errorf("%d required", key)
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
