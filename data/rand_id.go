// Copyright 2022 Block, Inc.

package data

import (
	"github.com/rs/xid"

	"github.com/square/finch"
)

type Xid struct {
	id    Id
	trxNo uint
	val   string
}

var _ Generator = &Xid{}

func NewXid(id Id) *Xid {
	// Set default scope if not explicitly configured
	if id.Scope == "" {
		id.Scope = finch.SCOPE_TRX
	}
	return &Xid{
		id: id,
	}
}

func (g *Xid) Id() Id                     { return g.id }
func (g *Xid) Format() string             { return "'%s'" }
func (g *Xid) Scan(any interface{}) error { return nil }

func (g *Xid) Copy(r finch.RunLevel) Generator {
	return NewXid(g.id.Copy(r))
}

func (g *Xid) Values(c finch.ExecCount) []interface{} {
	switch g.id.Scope {
	case finch.SCOPE_TRX:
		if c[finch.TRX] > g.trxNo { // new trx
			g.trxNo = c[finch.TRX]
			g.val = xid.New().String()
		}
	default:
		g.val = xid.New().String()
	}
	return []interface{}{g.val}
}
