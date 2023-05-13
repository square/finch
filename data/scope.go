// Copyright 2023 Block, Inc.

package data

import (
	"fmt"

	"github.com/square/finch"
)

// ExecCount is an array of counters for each scope const below. Each Client
// increments the counts as it executes. ScopedGenerator uses the counts to
// determine when to call again the Generator that it wraps for new values in
// the new scope.
type ExecCount [5]uint

// These const index ExecCount.
const (
	STAGE byte = iota
	EXEC_GROUP
	CLIENT_GROUP
	ITER
	TRX
	//QUERY is not counted because every call to Values implies query += 1
)

// Key represents one data key (@d).
type Key struct {
	Name      string
	Trx       string // first seen
	Line      uint   // first seen
	Statement uint   // first seen
	Column    int
	Generator Generator `deep:"-"`
}

func (k Key) String() string {
	s := fmt.Sprintf("%s line %d statement %d", k.Trx, k.Line, k.Statement)
	if k.Column >= 0 {
		s += fmt.Sprintf(" column %d", k.Column)
	}
	return s
}

type Scope struct {
	Keys     map[string]Key            // original generators created in trx.Load()
	CopyOf   map[string]Generator      `deep:"-"` // current scope (copy) of @d
	CopiedAt map[string]finch.RunLevel // that created ^
}

func NewScope() *Scope {
	return &Scope{
		Keys:     map[string]Key{},
		CopyOf:   map[string]Generator{},
		CopiedAt: map[string]finch.RunLevel{},
	}
}

func (s *Scope) Copy(keyName string, rl finch.RunLevel) Generator {
	if keyName == "@PREV" {
		return nil
	}
	k, ok := s.Keys[keyName]
	if !ok {
		panic("key not loaded: " + keyName)
	}
	scope := k.Generator.Id().Scope

	switch scope {
	case "", finch.SCOPE_STATEMENT:
		return k.Generator.Copy(rl)
	case finch.SCOPE_GLOBAL:
		return k.Generator
	}

	prev := s.CopiedAt[keyName]
	cp := false
	switch scope {
	case finch.SCOPE_STAGE, finch.SCOPE_WORKLOAD:
		// Scopes stage == workload because currently there's no config.stage.iterations.
		// If that's added, then we'll need to add Scope.Workload uint and increment that
		// count whenever the stage is re-run.
		cp = rl.Stage != prev.Stage
	case finch.SCOPE_EXEC_GROUP:
		cp = rl.ExecGroup > prev.ExecGroup
	case finch.SCOPE_CLIENT_GROUP:
		cp = rl.ClientGroup > prev.ClientGroup
	case finch.SCOPE_CLIENT:
		cp = rl.Client > prev.Client
	case finch.SCOPE_TRX:
		cp = rl.Trx > prev.Trx || rl.Client > prev.Client
	default:
		panic("invalid scope: " + scope)
	}
	if cp {
		s.CopyOf[keyName] = k.Generator.Copy(rl)
		s.CopiedAt[k.Name] = rl
	}
	return s.CopyOf[keyName]
}

func (s *Scope) Reset() {
	for keyName := range s.Keys {
		if s.Keys[keyName].Generator.Id().Scope == finch.SCOPE_GLOBAL {
			continue
		}
		delete(s.Keys, keyName)
		delete(s.CopyOf, keyName)
		delete(s.CopiedAt, keyName)
	}
}

// --------------------------------------------------------------------------

// ScopedGenerator wraps a Generator to handle scopes higher that statement.
// This prevents each generator from needing to be aware of or implement
// scope-handling logic. Generators are wrapped automatically in factory.Make.
type ScopedGenerator struct {
	g     Generator     // real generator
	s     string        //   its scope
	vals  []interface{} //   its current value
	trxNo uint          // last trx number for trx scope
}

var _ Generator = &ScopedGenerator{}

func NewScopedGenerator(g Generator) *ScopedGenerator {
	return &ScopedGenerator{
		g: g,
		s: g.Id().Scope,
	}
}

func (g *ScopedGenerator) Id() Id                     { return g.g.Id() }
func (g *ScopedGenerator) Format() string             { return g.g.Format() }
func (g *ScopedGenerator) Scan(any interface{}) error { return g.g.Scan(any) }

func (g *ScopedGenerator) Copy(r finch.RunLevel) Generator {
	return NewScopedGenerator(g.g.Copy(r))
}

func (g *ScopedGenerator) Values(cnt ExecCount) []interface{} {
	switch g.s {
	// @todo implement other scopes
	case finch.SCOPE_TRX:
		if cnt[TRX] > g.trxNo { // new trx
			g.trxNo = cnt[TRX]
			g.vals = g.g.Values(cnt)
		}
	default: // QUERY
		g.vals = g.g.Values(cnt)
	}
	return g.vals
}
