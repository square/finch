// Copyright 2022 Block, Inc.

package data

import (
	"bytes"
	"database/sql"

	"github.com/square/finch"
)

type Column struct {
	id      Id
	col     string
	colType string
	r       bool
	v       interface{}
	b       *bytes.Buffer
}

var _ Generator = &Column{}
var _ sql.Scanner = &Column{}

func NewColumn(id Id, col, colType string) *Column {
	return &Column{
		id:      id,
		col:     col,
		colType: colType,
	}
}

func (g *Column) Id() Id {
	return g.id
}

func (g *Column) Scope() string {
	return finch.SCOPE_TRANSACTION
}

func (g *Column) Format() string {
	if g.colType == "n" {
		return "%v"
	}
	return "'%v'"
}

func (g *Column) Copy(clientNo int) Generator {
	return NewColumn(g.id.Copy(clientNo), g.col, g.colType)
}

func (g *Column) Values() []interface{} {
	if g.r {
		return []interface{}{g.b.String()}
	}
	return []interface{}{g.v}
}

func (g *Column) Scan(any interface{}) error {
	switch any.(type) {
	case []byte:
		g.r = true // is reference; copy bytes
		if g.b == nil {
			g.b = bytes.NewBuffer(make([]byte, len(any.([]byte))))
		}
		g.b.Reset()
		g.b.Write(any.([]byte))
	default:
		g.r = false // not reference; copy value
		g.v = any
	}
	return nil
}

var Noop = noop{}

type noop struct{}

var _ Generator = noop{}
var _ sql.Scanner = noop{}

func (g noop) Id() Id {
	return Id{}
}

func (g noop) Scope() string {
	return finch.SCOPE_GLOBAL
}

func (g noop) Format() string {
	return ""
}

func (g noop) Copy(clientNo int) Generator {
	return g
}

func (g noop) Values() []interface{} {
	return nil
}

func (g noop) Scan(any interface{}) error {
	return nil
}
