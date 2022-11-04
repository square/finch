// Copyright 2022 Block, Inc.

package data

import (
	"github.com/rs/xid"

	"github.com/square/finch"
)

type Xid struct {
	rows int
	vals []string
	i    int
	id   Id
}

var _ Generator = &Xid{}

func NewXid(id Id, rows int) *Xid {
	if rows <= 1 {
		return &Xid{id: id, rows: 1}
	}
	return &Xid{
		rows: rows,
		i:    rows,
		vals: make([]string, rows),
		id:   id,
	}
}

func (g *Xid) Id() Id {
	return g.id
}

func (g *Xid) Scope() string {
	if g.rows == 1 {
		return finch.SCOPE_GLOBAL
	}
	return finch.SCOPE_TRANSACTION
}

func (g *Xid) Format() string {
	return "'%s'"
}

func (g *Xid) Copy(clientNo int) Generator {
	return NewXid(g.id.Copy(clientNo), g.rows)
}

func (g *Xid) Values() []interface{} {
	if g.rows == 1 {
		return []interface{}{xid.New().String()}
	}
	if g.i == g.rows {
		s := xid.New().String()
		for i := 0; i < g.rows; i++ {
			g.vals[i] = s
		}
		g.i = 1
	} else {
		g.i++
	}
	return []interface{}{g.vals[g.i-1]}
}

func (g *Xid) Scan(any interface{}) error {
	return nil
}
