// Copyright 2023 Block, Inc.

package data

import (
	"fmt"

	"github.com/square/finch"
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
	// Don't copy @PREV because the previous generator will return 2 values
	if keyName == "@PREV" {
		return nil
	}

	k, ok := s.Keys[keyName]
	if !ok {
		panic("key not loaded: " + keyName) // panic because input already validated
	}

	// Copy the data generator if its configured scope has changed
	scope := k.Generator.Id().Scope // configured scope
	prev := s.CopiedAt[keyName]     // last time we say this @d
	cp := false                     // scope has changed; copy data generator
	switch scope {
	case finch.SCOPE_VALUE:
		cp = true // every value is new, so always copy
	case "", finch.SCOPE_STATEMENT:
		cp = rl.Query > prev.Query || rl.Trx > prev.Trx || rl.Client > prev.Client
	case finch.SCOPE_TRX:
		cp = rl.Trx > prev.Trx || rl.Client > prev.Client
	case finch.SCOPE_CLIENT:
		cp = rl.Client > prev.Client
	default:
		panic("invalid scope: " + scope) // panic because input already validated
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

// RunCount is an array of ITER and TRX counters. Each Client increments the
// counts as it executes. ScopedGenerator uses the counts to determine when to
// call the real Generator that it wraps for new value.
//
// Statements are not counted because every call to Generator.Values is +1 statement.
// Only trx and iter are counted because the other higher-level scopes haven't
// been needed and it's not clear when they should increment.
type RunCount [3]uint

const (
	ITER byte = iota
	TRX
	STATEMENT
)

var runlevelNumber = map[string]byte{
	//finch.SCOPE_GLOBAL:       GLOBAL,
	//finch.SCOPE_STAGE:        STAGE,
	//finch.SCOPE_WORKLOAD:     WORKLOAD,
	//finch.SCOPE_EXEC_GROUP:   EXEC_GROUP,
	//finch.SCOPE_CLIENT_GROUP: CLIENT_GROUP,
	finch.SCOPE_CLIENT:    ITER,
	finch.SCOPE_TRX:       TRX,
	finch.SCOPE_STATEMENT: STATEMENT,
}

// ScopedGenerator wraps a Generator to handle scopes higher that statement.
// This prevents each generator from needing to be aware of or implement
// scope-handling logic. Generators are wrapped automatically in factory.Make.
type ScopedGenerator struct {
	g    Generator     // real generator
	sno  byte          //   its scope (byte)
	vals []interface{} //   its current value
	last RunCount      //   last time was run
}

var _ Generator = &ScopedGenerator{}

func NewScopedGenerator(g Generator) *ScopedGenerator {
	return &ScopedGenerator{
		g:    g,
		sno:  runlevelNumber[g.Id().Scope],
		last: RunCount{},
	}
}

func (g *ScopedGenerator) Id() Id                     { return g.g.Id() }
func (g *ScopedGenerator) Format() string             { return g.g.Format() }
func (g *ScopedGenerator) Scan(any interface{}) error { return g.g.Scan(any) }

func (g *ScopedGenerator) Copy(r finch.RunLevel) Generator {
	return NewScopedGenerator(g.g.Copy(r))
}

func (g *ScopedGenerator) Values(cnt RunCount) []interface{} {
	if cnt[g.sno] > g.last[g.sno] {
		g.last[g.sno] = cnt[g.sno]
		g.vals = g.g.Values(cnt)
	}
	return g.vals
}
