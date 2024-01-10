// Copyright 2024 Block, Inc.

package data

import (
	"fmt"
	"strings"

	"github.com/rs/xid"

	"github.com/square/finch"
)

// Xid implments the xid data generator.
type Xid struct {
	val string
}

var _ Generator = &Xid{}

func NewXid() *Xid {
	return &Xid{}
}

func (g *Xid) Name() string               { return "xid" }
func (g *Xid) Format() (uint, string)     { return 1, "'%s'" }
func (g *Xid) Scan(any interface{}) error { return nil }

func (g *Xid) Copy() Generator {
	return NewXid()
}

func (g *Xid) Values(c RunCount) []interface{} {
	return []interface{}{xid.New().String()}
}

// --------------------------------------------------------------------------

// ClientId implments the client-id data generator.
type ClientId struct {
	ids []byte
}

var _ Generator = &ClientId{}

func NewClientId(params map[string]string) (*ClientId, error) {
	ids := []byte{}
	if len(params) == 0 {
		ids = append(ids, CLIENT) // just client ID by default
	} else {
		csvIds := params["ids"]
		if csvIds != "" {
			runlevels := strings.Split(csvIds, ",")
			for _, r := range runlevels {
				switch r {
				case finch.SCOPE_STATEMENT:
					ids = append(ids, STATEMENT)
				case finch.SCOPE_TRX:
					ids = append(ids, TRX)
				case finch.SCOPE_ITER:
					ids = append(ids, ITER)
				case "conn":
					ids = append(ids, CONN)
				case finch.SCOPE_CLIENT:
					ids = append(ids, CLIENT)
				case finch.SCOPE_CLIENT_GROUP:
					ids = append(ids, CLIENT_GROUP)
				case finch.SCOPE_EXEC_GROUP:
					ids = append(ids, EXEC_GROUP)
				default:
					return nil, fmt.Errorf("invalid scope: %s", r)
				}
			}
		}
	}
	return &ClientId{ids: ids}, nil
}

func (g *ClientId) Name() string               { return "client-id" }
func (g *ClientId) Format() (uint, string)     { return uint(len(g.ids)), "%d" }
func (g *ClientId) Scan(any interface{}) error { return nil }
func (g *ClientId) Copy() Generator            { return &ClientId{ids: g.ids} }

func (g *ClientId) Values(rc RunCount) []interface{} {
	// This data generator can be shared (e.g. client-group scope), so the
	// caller can be a different client each time, so don't save the last value
	// and don't return something like g.val (a struct field) because Go slices
	// are pointers, so each call would overwrite the return value of the previous.
	val := make([]interface{}, len(g.ids)) // a new slice each call
	for i := range g.ids {
		val[i] = rc[g.ids[i]]
	}
	return val
}
