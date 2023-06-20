// Copyright 2023 Block, Inc.

package data_test

import (
	"sort"
	"testing"

	"github.com/go-test/deep"

	"github.com/square/finch"
	"github.com/square/finch/data"
)

func TestInteger_Int(t *testing.T) {
	finch.Debugging = true
	g, _ := data.NewInt(data.Id{}, map[string]string{
		"max": "1000",
	})
	r := data.RunCount{}

	got := []int{}
	for i := 0; i < 1000; i++ {
		v1 := g.Values(r)
		if len(v1) != 1 {
			t.Fatalf("got %d values, expected 1: %v", len(v1), v1)
		}
		got = append(got, int(v1[0].(int64)))
	}
	sort.Ints(got)
	n := got[0]
	if n < 1 {
		t.Errorf("lower bound < 1: %v", n)
	}
	n = got[len(got)-1] // amx
	if n > 1000 {
		t.Errorf("upper bound > max 1000: %v", n)
	}

	// Copy should be identical except Id
	deep.CompareUnexportedFields = true
	c := g.Copy(finch.RunLevel{Query: 2})
	if diff := deep.Equal(g, c); diff != nil {
		t.Error(diff)
	}
	deep.CompareUnexportedFields = false
}

func TestInteger_AutoInc(t *testing.T) {
	g, _ := data.NewAutoInc(data.Id{}, nil)
	r := data.RunCount{}

	for i := 1; i <= 3; i++ { // 1, 2, 3
		v1 := g.Values(r)
		if len(v1) != 1 {
			t.Fatalf("got %d values, expected 1: %v", len(v1), v1)
		}
		s1 := v1[0].(uint64)
		if s1 != uint64(i) {
			t.Errorf("got %v, expected %d", v1[0], i)
		}
	}

	// start=5: first value is 5+1 (6), then 6+1 (7)
	g, _ = data.NewAutoInc(data.Id{}, map[string]string{"start": "5"})
	for i := 6; i <= 7; i++ { // 6, 7
		v1 := g.Values(r)
		if len(v1) != 1 {
			t.Fatalf("got %d values, expected 1: %v", len(v1), v1)
		}
		s1 := v1[0].(uint64)
		if s1 != uint64(i) {
			t.Errorf("got %v, expected %d", v1[0], i)
		}
	}

	// step=2
	g, _ = data.NewAutoInc(data.Id{}, map[string]string{"step": "2"})
	for i := 1; i <= 2; i++ { // 2, 4
		v1 := g.Values(r)
		if len(v1) != 1 {
			t.Fatalf("got %d values, expected 1: %v", len(v1), v1)
		}
		s1 := v1[0].(uint64)
		if s1 != (uint64(i) * 2) {
			t.Errorf("got %v, expected %d", v1[0], i*2)
		}
	}

	// start=10 step=2
	g, _ = data.NewAutoInc(data.Id{}, map[string]string{"start": "10", "step": "2"})
	for _, i := range []uint64{12, 14} {
		v1 := g.Values(r)
		if len(v1) != 1 {
			t.Fatalf("got %d values, expected 1: %v", len(v1), v1)
		}
		s1 := v1[0].(uint64)
		if s1 != i {
			t.Errorf("got %v, expected %d", v1[0], i)
		}
	}
}

func TestInteger_IntRange(t *testing.T) {
	// Default is [1, 100000] with size 100
	g, _ := data.NewIntRange(data.Id{}, map[string]string{})
	r := data.RunCount{}

	got := [][]int64{}
	for i := 0; i < 1000; i++ {
		v1 := g.Values(r)
		if len(v1) != 2 {
			t.Fatalf("got %d values, expected 2: %v", len(v1), v1)
		}
		got = append(got, []int64{v1[0].(int64), v1[1].(int64)})
	}
	for _, pair := range got {
		if pair[1] <= pair[0] {
			t.Errorf("pair not ordered: %v", pair)
		}
		if pair[0] < 1 {
			t.Errorf("lower bound < 1: %v", pair)
		}
		if pair[1] > finch.ROWS {
			t.Errorf("upper bound > %d: %v", finch.ROWS, pair)
		}
		if n := pair[1] - pair[0] + 1; n < 1 || n > 100 {
			t.Errorf("range %d, expected 100 or less: %v", n, pair)
		}
	}
}

func TestInteger_IntRangeSeq(t *testing.T) {
	g, _ := data.NewIntRangeSeq(data.Id{}, map[string]string{
		"begin": "1",
		"end":   "9",
		"size":  "3",
	})
	r := data.RunCount{}

	got := [][]int64{}
	for i := 0; i < 4; i++ {
		v1 := g.Values(r)
		if len(v1) != 2 {
			t.Fatalf("got %d values, expected 2: %v", len(v1), v1)
		}
		got = append(got, []int64{v1[0].(int64), v1[1].(int64)})
	}
	expect := [][]int64{
		{1, 3},
		{4, 6},
		{7, 9},
		{1, 3},
	}
	if diff := deep.Equal(got, expect); diff != nil {
		t.Error(diff)
		t.Errorf("%+v", got)
	}

	// Short chunk at end
	g, _ = data.NewIntRangeSeq(data.Id{}, map[string]string{
		"begin": "1",
		"end":   "9",
		"size":  "4",
	})
	got = [][]int64{}
	for i := 0; i < 4; i++ {
		v1 := g.Values(r)
		if len(v1) != 2 {
			t.Fatalf("got %d values, expected 2: %v", len(v1), v1)
		}
		got = append(got, []int64{v1[0].(int64), v1[1].(int64)})
	}
	expect = [][]int64{
		{1, 4},
		{5, 8},
		{9, 9}, // short chunk
		{1, 4},
	}
	if diff := deep.Equal(got, expect); diff != nil {
		t.Error(diff)
		t.Errorf("%+v", got)
	}
}

func TestInteger_IntGaps(t *testing.T) {
	g, err := data.NewIntGaps(data.Id{}, map[string]string{
		"min": "1",
		"max": "100",
		"p":   "20", // percent
	})
	if err != nil {
		t.Fatal(err)
	}
	r := data.RunCount{}
	v := map[int64]bool{}
	for i := 0; i < 1000; i++ {
		v1 := g.Values(r)
		if len(v1) != 1 {
			t.Fatalf("got %d values, expected 1: %v", len(v1), v1)
		}
		v[v1[0].(int64)] = true
	}
	got := make([]int, 0, len(v))
	for k := range v {
		got = append(got, int(k))
	}
	sort.Ints(got)
	expect := []int{1, 6, 11, 16, 21, 27, 32, 37, 42, 47, 53, 58, 63, 68, 73, 79, 84, 89, 94, 100}
	if diff := deep.Equal(got, expect); diff != nil {
		t.Errorf("%v: %v", diff, got)
	}
	if len(v) < 19 || len(v) > 21 {
		t.Errorf("got %d unique values, expected 19, 20, or 21 (20%% of 100)", len(v))
	}
}
