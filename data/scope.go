// Copyright 2024 Block, Inc.

package data

import (
	"fmt"
	"sync"

	"github.com/square/finch"
)

// Id identifies the scope and copy number of a @d in a ScopedGenerator.
// It's only for debugging; it's not used to determine anything. The CopyNo
// is especially important because ScopedGenerator are copied from  real
// generators (from the pkg Make func), and they must be copied at the correct
// scope/run level, else @d will generate values at the wrong time/place.
type Id struct {
	finch.RunLevel
	Scope   string // finch.SCOPE_*
	Type    string // str-fill-az
	DataKey string // @d
	CopyNo  uint   // 1..N
}

func (id Id) String() string {
	return fmt.Sprintf("%s %s(%d)/%s %s", id.RunLevel, id.DataKey, id.CopyNo, id.Scope, id.Type)
}

// Key represents a data key (@d). These are parsed in trx/trx.go as part of a
// Scope struct (below) because all keys are scoped and copied baseed on scope.
// Therefore, Key.Generator is the original data generator, also created in
// trx/trx.go (search that file for data.Make). In workload/workload.go,
// Scope.Copy is called to create a scoped key: copy of the original data generator
// based on the key scope configured in the stage file that trx.go passed to
// data.Make earlier. The original data generator is never called; it's only
// used to create scoped keys.
type Key struct {
	Name      string    // @d
	Trx       string    // trx (file) name
	Line      uint      // line number in trx file
	Statement uint      // statement number (1-indexed) in trx file
	Column    int       // -1: none, 0: insert ID, >=1: column
	Scope     string    // finch.SCOPE_*
	Generator Generator `deep:"-"` // original (copy 0) from which others are copied
}

func (k Key) String() string {
	s := fmt.Sprintf("%s line %d statement %d", k.Trx, k.Line, k.Statement)
	if k.Column >= 0 {
		s += fmt.Sprintf(" column %d", k.Column)
	}
	return s
}

// Scope tracks each Key to create a scoped key copy (in Scope.Copy). Only one
// Scope is created in trx/trx.go for all data keys in all trx files. Then
// workload/workload.go calls Scope.Copy to create scoped keys for however the
// trx are assigned to exec groups, client groups, and clients (details that
// workload knows and handles).
type Scope struct {
	Keys      map[string]Key              // original generators created in trx.Load()
	CopyOf    map[string]*ScopedGenerator `deep:"-"` // current scope (copy) of @d
	CopiedAt  map[string]finch.RunLevel   // that created ^
	CopyCount map[string]uint             `deep:"-"`
	noop      *ScopedGenerator
}

func NewScope() *Scope {
	return &Scope{
		Keys:      map[string]Key{},
		CopyOf:    map[string]*ScopedGenerator{},
		CopiedAt:  map[string]finch.RunLevel{},
		CopyCount: map[string]uint{},
	}
}

func (s *Scope) Copy(keyName string, rl finch.RunLevel) *ScopedGenerator {
	// Don't copy @PREV because the previous generator will return multiple values
	if keyName == "@PREV" {
		return nil
	}

	k, ok := s.Keys[keyName]
	if !ok {
		panic("key not loaded: " + keyName) // panic because input already validated
	}

	if k.Name == finch.NOOP_COLUMN {
		// Only need 1 global no-op generator
		if s.noop == nil {
			s.noop = NewScopedGenerator(Id{Type: Noop.Name(), Scope: finch.SCOPE_GLOBAL}, Noop)
		}
		return s.noop
	}

	// If scope wasn't explicitly configured and generator didn't set its default scope,
	// then set default statement scope to ensure Id.Scope is always set.
	if k.Scope == "" {
		switch k.Generator.Name() {
		case "column":
			k.Scope = finch.SCOPE_TRX
		default:
			k.Scope = finch.SCOPE_STATEMENT
		}
		s.Keys[keyName] = k
		finch.Debug("%s: defaults to %s scope", k.Name, k.Scope)
	}

	prev := s.CopiedAt[keyName] // last time we saw this @d
	if rl.GreaterThan(prev, k.Scope) {
		s.CopyCount[keyName] += 1
		id := Id{
			RunLevel: rl,
			Scope:    k.Scope,
			Type:     k.Generator.Name(),
			DataKey:  keyName,
			CopyNo:   s.CopyCount[keyName],
		}
		s.CopyOf[keyName] = NewScopedGenerator(id, k.Generator.Copy())
		s.CopiedAt[k.Name] = rl
	}
	return s.CopyOf[keyName]
}

func (s *Scope) Reset() {
	for keyName, k := range s.Keys {
		if k.Scope == finch.SCOPE_STAGE || k.Scope == finch.SCOPE_GLOBAL {
			continue
		}
		finch.Debug("delete %s (scope: %s)", keyName, k.Scope)
		delete(s.Keys, keyName)
		delete(s.CopyOf, keyName)
		delete(s.CopiedAt, keyName)
	}
}

// --------------------------------------------------------------------------

// ScopedGenerator wraps a real Generator to handle scoped value generation
// when called by a Client. The factory Make func in this pkg returns real
// generators that are scope-agnostic. Then workload.Allocator.Clients calls
// Scope.Copy to scope each real generator by wrapping it in a ScopedGenerator.
// Then the workload allocator assigns either ScopedGenerator.Values (normal
// case) or ScopedGenerator.Call to each @d input--the Client doesn't know or
// care which--to handle either scoped value generator or explicit calls like
// @d().
type ScopedGenerator struct {
	id           Id                     // identify this copy of the real Generator for debugging
	g            Generator              // real Generator:
	sno          byte                   //   scope number in RunCount (if singleClient == true)
	last         RunCount               //   last time value was generated
	vals         []interface{}          //   last value
	singleClient bool                   // Single client scopes (typical): STATEMENT, TRX, ITER, CILENT
	oneTime      bool                   // One time scopes: STAGE and GLOBAL
	cgMux        *sync.RWMutex          // Multi client: client-group, exec-group, workload
	cgIter       map[uint]uint          //   last client iter
	cgVals       map[uint][]interface{} //   last client value
}

var _ Generator = &ScopedGenerator{}

func NewScopedGenerator(id Id, g Generator) *ScopedGenerator {
	s := &ScopedGenerator{
		id: id,
		g:  g, // real Generator
	}

	switch id.Scope {
	// Single client scopes (most common)
	case finch.SCOPE_STATEMENT, finch.SCOPE_TRX, finch.SCOPE_ITER, finch.SCOPE_CLIENT:
		s.singleClient = true
		s.sno = byte(finch.RunLevelNumber(id.Scope)) // these match, see finch.runlevelNumber comment
	// Multi client scopes: iter = each <client, iter>
	case finch.SCOPE_CLIENT_GROUP, finch.SCOPE_EXEC_GROUP, finch.SCOPE_WORKLOAD:
		s.cgMux = &sync.RWMutex{}
		s.cgIter = map[uint]uint{}
		s.cgVals = map[uint][]interface{}{}
	// One time scopes
	case finch.SCOPE_STAGE, finch.SCOPE_GLOBAL:
		s.oneTime = true
	case finch.SCOPE_VALUE:
		// Nothing to set
	default:
		panic("invalid scope: " + id.Scope)
	}

	return s
}

func (s *ScopedGenerator) Name() string               { return s.g.Name() }
func (s *ScopedGenerator) Id() Id                     { return s.id }
func (s *ScopedGenerator) Format() (uint, string)     { return s.g.Format() }
func (s *ScopedGenerator) Scan(any interface{}) error { return s.g.Scan(any) }

func (s *ScopedGenerator) Copy() Generator {
	panic("cannot copy ScopedGenerator") // only real Generator is copied
}

func (s *ScopedGenerator) Call(cnt RunCount) []interface{} {
	/*
		This func called in performance critical path: Client.Run.
		Don't debug or call anything slow/superfluous.
	*/
	if s.cgMux != nil { // multi client
		clientNo := cnt[CLIENT]
		v := s.g.Values(cnt)
		s.cgMux.Lock()
		s.cgIter[clientNo] = cnt[ITER]
		s.cgVals[clientNo] = v
		s.cgMux.Unlock()
		return v
	}
	s.last[s.sno] = cnt[s.sno] // save last run counter value
	s.vals = s.g.Values(cnt)   // generate new data value
	return s.vals
}

func (s *ScopedGenerator) Values(cnt RunCount) []interface{} {
	/*
		This func called in performance critical path: Client.Run.
		Don't debug or call anything slow/superfluous.
	*/

	// Typical case: single client scopes
	// Generate a new data value (g.g.Values) when the run counter
	// for this scope has incremented (is greater than last value)
	if s.singleClient {
		if cnt[s.sno] > s.last[s.sno] {
			return s.Call(cnt) // new value
		}
		return s.vals // old value (scope hasn't changed)
	}

	// Multi client scopes: CLIENT_GROUP, EXEC_GROUP, WORKLOAD
	if s.cgMux != nil {
		clientNo := cnt[CLIENT]
		s.cgMux.RLock()
		prevIter, ok := s.cgIter[clientNo]
		if !ok || cnt[ITER] > prevIter {
			s.cgMux.RUnlock()
			return s.Call(cnt)
		}
		v := s.cgVals[clientNo]
		s.cgMux.RUnlock()
		return v
	}

	// One time scopes: STAGE and GLOBAL
	if s.oneTime {
		// @todo guard with mux
		if s.vals != nil {
			return s.Call(cnt)
		}
		return s.vals
	}

	// VALUE scope
	return s.g.Values(cnt)
}

// RunCount counts execution (or changes) at each level in the order defined
// by the const below: STATEMENT and up the run levels. Each Client maintains
// a RunCount that is used by ScopedGenerator.Values to determine when it's
// time to generate a new value based on the scope of the @d.
type RunCount [8]uint

const (
	// Counters
	STATEMENT byte = iota
	TRX
	ITER
	CONN
	// From Client.RunLevel in case a data.Generator wants to know
	CLIENT
	CLIENT_GROUP
	EXEC_GROUP
	STAGE
)
