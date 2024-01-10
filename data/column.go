// Copyright 2024 Block, Inc.

package data

import (
	"bytes"
	"database/sql"

	"github.com/square/finch"
)

// Column is a special Generator that is used to save (Scan) values from rows
// or insert ID, then return those values (Value) to other statements.
type Column struct {
	quoteValue bool
	val        interface{}
	bytes      *bytes.Buffer
	useBytes   bool
}

var _ Generator = &Column{}
var _ sql.Scanner = &Column{}

func NewColumn(params map[string]string) *Column {
	return &Column{
		quoteValue: finch.Bool(params["quote-value"]),
	}
}

func (g *Column) Name() string { return "column" }

func (g *Column) Format() (uint, string) {
	if g.quoteValue {
		return 1, "'%v'"
	}
	return 1, "%v"
}

func (g *Column) Copy() Generator {
	return &Column{
		quoteValue: g.quoteValue,
	}
}

func (g *Column) Scan(any interface{}) error {
	// @todo column type won't change, so maybe sync.Once to set val or bytes
	// will make this more efficient?
	switch any.(type) {
	case []byte:
		g.useBytes = true // is reference; copy bytes
		if g.bytes == nil {
			g.bytes = bytes.NewBuffer(make([]byte, len(any.([]byte))))
		}
		g.bytes.Reset()
		g.bytes.Write(any.([]byte))
	default:
		g.useBytes = false // not reference; copy value
		g.val = any
	}
	return nil
}

func (g *Column) Values(_ RunCount) []interface{} {
	if g.useBytes {
		return []interface{}{g.bytes.String()}
	}
	return []interface{}{g.val}
}

// --------------------------------------------------------------------------

var Noop = noop{}

type noop struct{}

var _ Generator = Noop
var _ sql.Scanner = Noop

func (g noop) Name() string                    { return "no-op" }
func (g noop) Format() (uint, string)          { return 0, "" }
func (g noop) Copy() Generator                 { return Noop }
func (g noop) Values(_ RunCount) []interface{} { return nil }
func (g noop) Scan(any interface{}) error      { return nil }
