package data_test

import (
	"testing"

	"github.com/square/finch"
	"github.com/square/finch/data"
)

func TestXid_TrxScope(t *testing.T) {
	g := data.NewXid(data.Id{
		Scope:   finch.SCOPE_TRX,
		Type:    "xid",
		DataKey: "@d",
	})

	r := finch.ExecCount{}
	r[finch.TRX] = 1

	v1 := g.Values(r)
	v2 := g.Values(r)

	if len(v1) != 1 || len(v2) != 1 {
		t.Fatalf("%d and %d values, expected 1 value from each call to Values()", len(v1), len(v2))
	}

	if v1[0].(string) != v2[0].(string) {
		t.Errorf("different values for same trx, expect same: %s != %s", v1, v2)
	}

	// Next trx should cause new value
	r[finch.TRX] += 1
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
