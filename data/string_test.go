// Copyright 2023 Block, Inc.

package data_test

import (
	"strconv"
	"testing"

	"github.com/square/finch"
	"github.com/square/finch/data"
)

func TestString_StrFillAz(t *testing.T) {
	lens := []string{"1", "10", "100", "1000", "10000", "100000", "1000000"}
	for _, strlen := range lens {
		g, _ := data.NewStrFillAz(
			data.Id{
				Scope:   finch.SCOPE_STATEMENT,
				Type:    "str-az",
				DataKey: "@d",
			},
			map[string]string{
				"len": strlen,
			},
		)

		r := data.RunCount{}

		v1 := g.Values(r)
		if len(v1) != 1 {
			t.Fatalf("len=%s: got %d values, expected 1: %v", strlen, len(v1), v1)
		}
		s1 := v1[0].(string)
		n, _ := strconv.Atoi(strlen)
		if len(s1) != n {
			t.Errorf("len=%s: got len %d, expected %s: %s", strlen, len(s1), strlen, s1)
		}
	}
}
