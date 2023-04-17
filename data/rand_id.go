// Copyright 2023 Block, Inc.

package data

import (
	"github.com/rs/xid"

	"github.com/square/finch"
)

type Xid struct {
	id  Id
	val string
}

var _ Generator = &Xid{}

func NewXid(id Id) *Xid {
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
	return []interface{}{xid.New().String()}
}
