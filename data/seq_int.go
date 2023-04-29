// Copyright 2023 Block, Inc.

package data

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/square/finch"
)

type IntSequence struct {
	id     Id
	min    int64
	max    int64
	size   int64
	n      int64
	params map[string]string
	*sync.Mutex
}

var _ Generator = &IntSequence{}

func NewIntSequence(id Id, params map[string]string) (*IntSequence, error) {
	g := &IntSequence{
		id:     id,
		min:    1,
		max:    10000, // 100,000 by default
		size:   10,
		n:      1,
		params: params,
		Mutex:  &sync.Mutex{},
	}
	if err := int64From(params, "size", &g.size, false); err != nil {
		return nil, err
	}
	if err := int64From(params, "min", &g.min, false); err != nil {
		return nil, err
	}
	g.n = g.min
	if err := int64From(params, "max", &g.max, false); err != nil {
		return nil, err
	}
	if g.min >= g.max {
		return nil, fmt.Errorf("invalid int sequence: max not greater than min")
	}
	if g.size > (g.max - g.min) {
		return nil, fmt.Errorf("invalid int sequence: size > max - min")
	}
	finch.Debug("%s: %d values from %d to %d", g.id, g.size, g.min, g.max)
	return g, nil
}

func (g *IntSequence) Id() Id { return g.id }

func (g *IntSequence) Format() string { return "%d" }

func (g *IntSequence) Copy(r finch.RunLevel) Generator {
	c, _ := NewIntSequence(g.id.Copy(r), g.params)
	return c
}

func (g *IntSequence) Values(_ ExecCount) []interface{} {
	g.Lock()
	lower := g.n
	upper := g.n + g.size
	if upper <= g.max {
		g.n += g.size // next chunk
	} else {
		upper = g.max
		finch.Debug("%s: reached %d, resetting to %d", g.id, g.max, g.min)
		g.n = g.min // reset
	}
	g.Unlock()
	return []interface{}{lower, upper}
}

func (g *IntSequence) Scan(any interface{}) error {
	return nil
}

// --------------------------------------------------------------------------

type IncUint64 struct {
	i  uint64
	id Id
}

var _ Generator = &IncUint64{}

func NewIncUint64(id Id, params map[string]string) *IncUint64 {
	return &IncUint64{id: id}
}

func (g *IncUint64) Id() Id { return g.id }

func (g *IncUint64) Format() string { return "%d" }

func (g *IncUint64) Copy(r finch.RunLevel) Generator { return NewIncUint64(g.id.Copy(r), nil) }

func (g *IncUint64) Values(_ ExecCount) []interface{} {
	return []interface{}{atomic.AddUint64(&g.i, 1)}
}

func (g *IncUint64) Scan(any interface{}) error { return nil }
