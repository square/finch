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

func (s *Scope) Copy(keyName string, runlevel finch.RunLevel) Generator {
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
		return k.Generator.Copy(runlevel)
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
		cp = runlevel.Stage != prev.Stage
	case finch.SCOPE_EXEC_GROUP:
		cp = runlevel.ExecGroup > prev.ExecGroup
	case finch.SCOPE_CLIENT_GROUP:
		cp = runlevel.ClientGroup > prev.ClientGroup
	case finch.SCOPE_CLIENT:
		cp = runlevel.Client > prev.Client
	case finch.SCOPE_TRX:
		cp = runlevel.Trx > prev.Trx
	default:
		panic("invalid scope: " + scope)
	}
	if cp {
		s.CopyOf[keyName] = k.Generator.Copy(runlevel)
		s.CopiedAt[k.Name] = runlevel
	}
	return s.CopyOf[keyName]
}

func (s *Scope) Reset() {
	for keyName := range s.Keys {
		if s.Keys[keyName].Generator.Id().Scope == finch.SCOPE_GLOBAL {
			continue
		}
		delete(s.Keys, keyName)
	}
}
