// Copyright 2024 Block, Inc.

package data_test

import (
	"testing"

	"github.com/go-test/deep"
	"github.com/square/finch"
	"github.com/square/finch/data"
)

func TestScope_Trx(t *testing.T) {
	keyName := "@d"
	g, _ := data.NewAutoInc(nil)

	r := finch.RunLevel{
		Stage:         1,
		StageName:     "benchmark",
		ExecGroup:     1,
		ExecGroupName: "test-execgrp",
		ClientGroup:   1,
		Client:        1,
		Trx:           1,
		TrxName:       "test-trx",
		Query:         1,
	}

	scope := data.NewScope()
	scope.Keys[keyName] = data.Key{
		Name:      keyName,
		Scope:     finch.SCOPE_TRX,
		Trx:       "test-trx",
		Statement: 1,
		Column:    -1,
		Generator: g,
	}

	// ----------------------------------------------------------------------
	// Client 1

	g1 := scope.Copy(keyName, r) // COPY 1
	id := g1.Id()
	if id.CopyNo != 1 {
		t.Errorf("got copy %d, expected 1: %+v", id.CopyNo, id)
	}

	// Trx hasn't changed, so data gen shouldn't change; should be first copy ^
	r.Query += 1
	g2 := scope.Copy(keyName, r)
	id = g2.Id()
	if id.CopyNo != 1 {
		t.Errorf("got copy %d, expected 1: %+v", id.CopyNo, id)
	}
	if g1 != g2 {
		t.Errorf("got differnet generators, expected same")
	}

	// Now trx changes, which should generate a new copy
	r.Trx += 1
	r.Query = 1                  // query count resets for new trx
	g3 := scope.Copy(keyName, r) // COPY 2
	id = g3.Id()
	if id.CopyNo != 2 {
		t.Errorf("got copy %d, expected 2 after trx change: %+v", id.CopyNo, id)
	}
	if g3 == g2 {
		t.Errorf("got same generator, expected different after trx change")
	}

	// Copy 2 should be recorded at the correct run level (i.e. trx 2, query 1)
	gotRL := scope.CopiedAt[keyName]
	expectRL := r
	expectRL.Trx = 2
	expectRL.Query = 1
	if diff := deep.Equal(gotRL, expectRL); diff != nil {
		t.Error(diff)
	}

	// Next stmt in new trx should should use its generator (copy 2)
	r.Query += 1
	g4 := scope.Copy(keyName, r)
	id = g4.Id()
	if id.CopyNo != 2 {
		t.Errorf("got copy %d, expected 2: %+v", id.CopyNo, id)
	}
	if g4 != g3 {
		t.Errorf("got differnet generators, expected same")
	}

	// Recorded run level shouldn't change unless/until generator changes
	// i.e. trx gen copy 2 still only copied at trx 2, query 1 (same as above)
	gotRL = scope.CopiedAt[keyName]
	if diff := deep.Equal(gotRL, expectRL); diff != nil {
		t.Error(diff)
	}

	// ----------------------------------------------------------------------
	// Client 2
	r.Client = 2
	r.Trx = 1
	r.Query = 1

	g5 := scope.Copy(keyName, r)
	id = g5.Id()
	if id.CopyNo != 3 {
		t.Errorf("got copy %d, expected 3 after client change: %+v", id.CopyNo, id)
	}
	if g5 == g4 {
		t.Errorf("got same generator, expected different after client change")
	}
}

func TestScope_PREV(t *testing.T) {
	// Test that a @PREV key is handled by using the previous key, as in
	//   SELECT c FROM t WHERE id BETWEEN @d AND @PREV
	// That's 2 statement inputs (values) but only 1 data gen. So copying
	// @PREV should return nil to signal "skip it; previous data gen will
	// return multiple values."
	keyName := "@d"
	g, _ := data.NewAutoInc(nil)
	r := finch.RunLevel{
		Client: 1,
		Trx:    1,
		Query:  1,
	}
	scope := data.NewScope()
	scope.Keys[keyName] = data.Key{
		Name:      keyName,
		Trx:       "test-trx",
		Statement: 1,
		Column:    -1,
		Generator: g,
	}
	g1 := scope.Copy(keyName, r)
	if g1 == nil {
		t.Fatal("got nil Generator, expected a value")
	}
	id := g1.Id()
	if id.CopyNo != 1 {
		t.Errorf("got copy %d, expected 1: %+v", id.CopyNo, id)
	}
	g2 := scope.Copy("@PREV", r) // should return nil
	if g2 != nil {
		t.Errorf("got Generator for @PREV, expected nil: %+v", g2)
	}
}
