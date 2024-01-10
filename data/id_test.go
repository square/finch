// Copyright 2024 Block, Inc.

package data_test

import (
	"testing"

	"github.com/go-test/deep"

	"github.com/square/finch"
	"github.com/square/finch/data"
)

func TestXid_TrxScope(t *testing.T) {
	g := data.NewScopedGenerator(
		data.Id{
			Scope:   finch.SCOPE_TRX,
			Type:    "xid",
			DataKey: "@d",
		},
		data.NewXid())

	r := data.RunCount{}
	r[data.TRX] = 1

	v1 := g.Values(r)
	v2 := g.Values(r)

	if len(v1) != 1 || len(v2) != 1 {
		t.Fatalf("%d and %d values, expected 1 value from each call to Values()", len(v1), len(v2))
	}

	if v1[0].(string) != v2[0].(string) {
		t.Errorf("different values for same trx, expect same: %s != %s", v1, v2)
	}

	// Next trx should cause new value
	r[data.TRX] += 1
	v3 := g.Values(r)
	v4 := g.Values(r)

	if len(v3) != 1 || len(v4) != 1 {
		t.Fatalf("%d and %d values, expected 1 value from each call to Values()", len(v3), len(v4))
	}

	if v3[0].(string) != v4[0].(string) {
		t.Errorf("different values for same trx, expect same: %s != %s", v3, v4)
	}

	if v3[0].(string) == v1[0].(string) {
		t.Errorf("trx 2 values == trx 1 values, expected different values: %s == %s", v3[0].(string), v1[0].(string))
	}
}

func TestClientId(t *testing.T) {
	rc := data.RunCount{
		1, 1, 1, 1, // couters
		5, 6, 7, 8, // client, cg, eg, stage
	}

	// With default (no params): returns just client id (5)
	g, err := data.NewClientId(nil)
	if err != nil {
		t.Fatal(err)
	}

	got := g.Values(rc)
	expect := []interface{}{uint(5)}
	if diff := deep.Equal(got, expect); diff != nil {
		t.Error(diff)
	}

	n, _ := g.Format()
	if n != 1 {
		t.Errorf("Format return n=%d, expected 1", n)
	}

	// With all 3 ids: client, client group, exec group
	g, err = data.NewClientId(map[string]string{"ids": "client,client-group,exec-group"})
	if err != nil {
		t.Fatal(err)
	}

	got = g.Values(rc)
	expect = []interface{}{uint(5), uint(6), uint(7)}
	if diff := deep.Equal(got, expect); diff != nil {
		t.Error(diff)
	}

	n, _ = g.Format()
	if n != 3 {
		t.Errorf("Format return n=%d, expected 3", n)
	}
}
