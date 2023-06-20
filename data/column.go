// Copyright 2023 Block, Inc.

package data

import (
	"bytes"
	"database/sql"

	"github.com/square/finch"
)

// Column implements the column data generator.
type Column struct {
	id     Id
	params map[string]string
	// --
	col          string
	colType      string
	val          interface{}
	bytes        *bytes.Buffer
	useBytes     bool
	rowsAffected bool // save/return sql.Result.RowsAffected
	rRows        int64
	insertId     bool // save/return sql.Result.LastInsertId
	rId          int64
}

var _ Generator = &Column{}
var _ sql.Scanner = &Column{}

func NewColumn(id Id, params map[string]string) *Column {
	// Set default scope if not explicitly configured
	if id.Scope == "" {
		id.Scope = finch.SCOPE_TRX
	}
	return &Column{
		id:           id,
		params:       params,
		col:          params["name"],
		colType:      params["type"],
		rowsAffected: finch.Bool(params["save-rows"]),
		insertId:     finch.Bool(params["save-insert-id"]),
	}
}

func (g *Column) Id() Id {
	return g.id
}

func (g *Column) Format() string {
	if g.colType == "n" {
		return "%v"
	}
	return "'%v'"
}

func (g *Column) Copy(r finch.RunLevel) Generator {
	return NewColumn(g.id.Copy(r), g.params)
}

func (g *Column) Values(_ RunCount) []interface{} {
	if g.insertId {
		return []interface{}{g.rId}
	}
	if g.rowsAffected {
		return []interface{}{g.rRows}
	}
	if g.useBytes {
		return []interface{}{g.bytes.String()}
	}
	return []interface{}{g.val}
}

func (g *Column) Scan(any interface{}) error {
	// sql.Result values from a write
	if g.insertId || g.rowsAffected {
		v := any.([]int64)
		g.rRows = v[0]
		g.rId = v[1]
		return nil
	}

	// Column value from SELECT
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

// --------------------------------------------------------------------------

var Noop = noop{}

type noop struct{}

var _ Generator = noop{}
var _ sql.Scanner = noop{}

func (g noop) Id() Id {
	return Id{
		Scope: finch.SCOPE_GLOBAL,
		Type:  "no-op",
	}
}

func (g noop) Format() string {
	return ""
}

func (g noop) Copy(r finch.RunLevel) Generator {
	return g
}

func (g noop) Values(_ RunCount) []interface{} {
	return nil
}

func (g noop) Scan(any interface{}) error {
	return nil
}
