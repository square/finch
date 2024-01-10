// Copyright 2024 Block, Inc.

package trx_test

import (
	"testing"

	"github.com/go-test/deep"

	"github.com/square/finch"
	"github.com/square/finch/config"
	"github.com/square/finch/data"
	"github.com/square/finch/trx"
)

var p = map[string]string{}

func TestLoad_001(t *testing.T) {
	// The most basic test: 1 query, 1 @d, nothing fancy.
	trxList := []config.Trx{
		{
			Name: "001.sql", // must set because we don't call Validate
			File: "../test/trx/001.sql",
			Data: map[string]config.Data{
				"id": {
					Generator: "int",
				},
			},
		},
	}

	scope := data.NewScope()
	got, err := trx.Load(trxList, scope, p)
	if err != nil {
		t.Fatal(err)
	}

	expect := &trx.Set{
		Order: []string{"001.sql"},
		Statements: map[string][]*trx.Statement{
			"001.sql": []*trx.Statement{
				{
					Trx:       "001.sql",
					Query:     "select c from t where id=%d",
					Inputs:    []string{"@id"},
					ResultSet: true,
					Calls:     []byte{0},
				},
			},
		},
		Data: &data.Scope{
			Keys: map[string]data.Key{
				"@id": data.Key{
					Name:      "@id",
					Trx:       "001.sql",
					Line:      1,
					Statement: 1,
					Column:    -1,
					Scope:     finch.SCOPE_STATEMENT,
				},
			},
			CopiedAt: map[string]finch.RunLevel{},
		},
		Meta: map[string]trx.Meta{
			"001.sql": {DDL: false},
		},
	}

	if diff := deep.Equal(got, expect); diff != nil {
		t.Error(diff)
		t.Logf("got: %#v", got)
	}
}

func TestLoad_002(t *testing.T) {
	// Basic "@d AND @PREV" test: @PREV causes no additional data gen, but it
	// should be interpolated into query like "%d AND %d", and then the 1 data gen
	// should return 2 values./
	trxList := []config.Trx{
		{
			Name: "002.sql", // must set because we don't call Validate
			File: "../test/trx/002.sql",
			Data: map[string]config.Data{
				"d": {
					Generator: "int",
				},
			},
		},
	}

	scope := data.NewScope()
	got, err := trx.Load(trxList, scope, p)
	if err != nil {
		t.Fatal(err)
	}

	expect := &trx.Set{
		Order: []string{"002.sql"},
		Statements: map[string][]*trx.Statement{
			"002.sql": []*trx.Statement{
				{
					Trx:       "002.sql",
					Query:     "SELECT c FROM t WHERE id BETWEEN %d AND %d",
					Inputs:    []string{"@d", "@PREV"},
					ResultSet: true,
					Calls:     []byte{0, 0},
				},
			},
		},
		Data: &data.Scope{
			Keys: map[string]data.Key{
				"@d": data.Key{
					Name:      "@d",
					Trx:       "002.sql",
					Line:      2,
					Statement: 1,
					Column:    -1,
					Scope:     finch.SCOPE_STATEMENT,
				},
				// No key for @PREV, but it is in Inputs ^
			},
			CopiedAt: map[string]finch.RunLevel{},
		},
		Meta: map[string]trx.Meta{
			"002.sql": {DDL: false},
		},
	}

	if diff := deep.Equal(got, expect); diff != nil {
		t.Error(diff)
		t.Logf("got: %#v", got)
	}
}

func TestLoad_003(t *testing.T) {
	// Basic column ref: one stmt saves a col (output), the other uses it as input
	trxList := []config.Trx{
		{
			Name: "003.sql", // must set because we don't call Validate
			File: "../test/trx/003.sql",
			Data: map[string]config.Data{
				"c": {
					Generator: "str-not-null",
					Params: map[string]string{
						"quote-value": "yes",
					},
				},
			},
		},
	}

	scope := data.NewScope()
	got, err := trx.Load(trxList, scope, p)
	if err != nil {
		t.Fatal(err)
	}

	expect := &trx.Set{
		Order: []string{"003.sql"},
		Statements: map[string][]*trx.Statement{
			"003.sql": []*trx.Statement{
				{
					Trx:       "003.sql",
					Query:     "select c from t1 where id=1",
					Inputs:    nil,
					Outputs:   []string{"@c"},
					ResultSet: true,
					// no Calls because this is an output column
				},
				{
					Trx:     "003.sql",
					Query:   "insert into t2 values ('%v')",
					Inputs:  []string{"@c"},
					Outputs: nil,
					Write:   true,
					Calls:   []byte{0},
				},
			},
		},
		Data: &data.Scope{
			Keys: map[string]data.Key{
				"@c": data.Key{
					Name:      "@c",
					Trx:       "003.sql",
					Line:      4,
					Statement: 1,
					Column:    0,
					Scope:     "",
				},
			},
			CopiedAt: map[string]finch.RunLevel{},
		},
		Meta: map[string]trx.Meta{
			"003.sql": {DDL: false},
		},
	}

	if diff := deep.Equal(got, expect); diff != nil {
		t.Error(diff)
		t.Logf("got: %#v", got)
	}
}

func TestLoad_copy3(t *testing.T) {
	// -- copy: 3 should yield 3x the same query. copy3-1.sql has the copy: 3
	// mod first, then a prepare mode. copy3-2.sql has the reverse. This is to
	// test that copy works with other mods in any order. Either way, same result:
	expect := &trx.Set{
		Order: []string{"copy3"},
		Statements: map[string][]*trx.Statement{
			"copy3": []*trx.Statement{
				{
					Trx:          "copy3",
					Query:        "select c from t where id=?",
					Inputs:       []string{"@id"},
					ResultSet:    true,
					Prepare:      true,
					PrepareMulti: 3,
					Calls:        []byte{0},
				},
				{
					Trx:          "copy3",
					Query:        "select c from t where id=?",
					Inputs:       []string{"@id"},
					ResultSet:    true,
					Prepare:      true,
					PrepareMulti: 0,
					Calls:        []byte{0},
				},

				{
					Trx:          "copy3",
					Query:        "select c from t where id=?",
					Inputs:       []string{"@id"},
					ResultSet:    true,
					Prepare:      true,
					PrepareMulti: 0,
					Calls:        []byte{0},
				},
			},
		},
		Data: &data.Scope{
			Keys: map[string]data.Key{
				"@id": data.Key{
					Name:      "@id",
					Trx:       "copy3",
					Line:      4,
					Statement: 1,
					Column:    -1,
					Scope:     finch.SCOPE_TRX,
				},
			},
			CopiedAt: map[string]finch.RunLevel{},
		},
		Meta: map[string]trx.Meta{
			"copy3": {DDL: false},
		},
	}

	trxList := []config.Trx{
		{
			Name: "copy3", // must set because we don't call Validate
			File: "../test/trx/copy3-1.sql",
			Data: map[string]config.Data{
				"id": {
					Generator: "int",
					Scope:     "trx",
				},
			},
		},
	}

	scope := data.NewScope()
	got, err := trx.Load(trxList, scope, p)
	if err != nil {
		t.Fatal(err)
	}
	if diff := deep.Equal(got, expect); diff != nil {
		t.Error(diff)
		t.Logf("got: %#v", got)
	}

	// ----------------------------------------------------------------------

	trxList = []config.Trx{
		{
			Name: "copy3",
			File: "../test/trx/copy3-2.sql",
			Data: map[string]config.Data{
				"id": {
					Generator: "int",
					Scope:     "trx",
				},
			},
		},
	}

	scope = data.NewScope()
	got, err = trx.Load(trxList, scope, p)
	if err != nil {
		t.Fatal(err)
	}
	if diff := deep.Equal(got, expect); diff != nil {
		t.Error(diff)
		t.Logf("got: %#v", got)
	}
}

func TestLoad_COPY_NUMBER(t *testing.T) {
	// /*!copy-number*/ should be replaced with the copy number
	expect := &trx.Set{
		Order: []string{"copyNo"},
		Statements: map[string][]*trx.Statement{
			"copyNo": []*trx.Statement{
				{
					Trx:       "copyNo",
					Query:     "select c from t1 where id=1",
					ResultSet: true,
				},
				{
					Trx:       "copyNo",
					Query:     "select c from t2 where id=1",
					ResultSet: true,
				},
			},
		},
		Data: &data.Scope{
			Keys:     map[string]data.Key{}, // no @d
			CopiedAt: map[string]finch.RunLevel{},
		},
		Meta: map[string]trx.Meta{
			"copyNo": {DDL: false},
		},
	}

	trxList := []config.Trx{
		{
			Name: "copyNo", // must set because we don't call Validate
			File: "../test/trx/copy-no.sql",
			Data: map[string]config.Data{
				"id": {
					Generator: "int",
					Scope:     "trx",
				},
			},
		},
	}

	scope := data.NewScope()
	got, err := trx.Load(trxList, scope, p)
	if err != nil {
		t.Fatal(err)
	}
	if diff := deep.Equal(got, expect); diff != nil {
		t.Error(diff)
		t.Logf("got: %#v", got)
	}
}

func TestLoad_RowScopeCSV(t *testing.T) {
	file := "rowscope-csv.sql"
	trxList := []config.Trx{
		{
			Name: file, // must set because we don't call Validate
			File: "../test/trx/" + file,
			Data: map[string]config.Data{
				"d": {
					Generator: "auto-inc",
				},
			},
		},
	}

	scope := data.NewScope()
	got, err := trx.Load(trxList, scope, p)
	if err != nil {
		t.Fatal(err)
	}

	expect := &trx.Set{
		Order: []string{file},
		Statements: map[string][]*trx.Statement{
			file: []*trx.Statement{
				{
					Trx:       file,
					Query:     "SELECT 1 -- (%d, %d %% 1000, '%d', '%d'), (%d, %d %% 1000, '%d', '%d')",
					Inputs:    []string{"@d", "@d", "@d", "@d", "@d", "@d", "@d", "@d"},
					Calls:     []byte{1, 0, 0, 0, 1, 0, 0, 0},
					ResultSet: true,
				},

				{
					Trx:       file,
					Query:     "SELECT 1 -- (%d, %d %% 1000, '%d', '%d'), (%d, %d %% 1000, '%d', '%d')",
					Inputs:    []string{"@d", "@d", "@d", "@d", "@d", "@d", "@d", "@d"},
					Calls:     []byte{1, 0, 0, 0, 1, 0, 0, 0},
					ResultSet: true,
				},
			},
		},
		Data: &data.Scope{
			Keys: map[string]data.Key{
				"@d": data.Key{
					Name:      "@d",
					Trx:       file,
					Line:      1,
					Statement: 1,
					Column:    -1,
					Scope:     finch.SCOPE_ROW,
				},
			},
			CopiedAt: map[string]finch.RunLevel{},
		},
		Meta: map[string]trx.Meta{
			file: {},
		},
	}

	if diff := deep.Equal(got, expect); diff != nil {
		t.Error(diff)
		t.Logf("got: %#v", got)
	}
}

func TestCalls(t *testing.T) {
	type test struct {
		inputs []string
		expect []byte
	}
	callTests := []test{
		{[]string{"@d"}, []byte{0}},
		{[]string{"@d", "@d"}, []byte{0, 0}},
		{[]string{"@d()"}, []byte{1}},
		{[]string{"@d()", "@d"}, []byte{1, 0}},
		{[]string{"@d", "@x", "@d()"}, []byte{0, 0, 1}},
		{[]string{"@d()", "@d()", "@d()"}, []byte{1, 1, 1}},
	}
	for _, c := range callTests {
		got := trx.Calls(c.inputs)
		if diff := deep.Equal(got, c.expect); diff != nil {
			t.Logf("%s -> %v", c.inputs, got)
			t.Error(diff)
		}
	}
}

func TestRowScope(t *testing.T) {
	s := map[string]bool{
		"@d": true,
		"@x": true,
	}
	type test struct {
		keys   map[string]bool
		in     string
		expect string
	}
	callTests := []test{
		{s, "(@d)", "(@d())"},
		{s, "(@d())", "(@d())"},
		{s, "(@d(), @d)", "(@d(), @d)"},
		{s, "(@d, @d)", "(@d(), @d)"},
		{s, "(@d, @x, @d)", "(@d(), @x(), @d)"},
		{s, "(@d, @x(), @d)", "(@d(), @x(), @d)"},
	}
	for _, c := range callTests {
		got := trx.RowScope(c.keys, c.in)
		if diff := deep.Equal(got, c.expect); diff != nil {
			t.Error(diff)
			t.Logf("%s -> %v", c.in, got)
		}
	}
}
