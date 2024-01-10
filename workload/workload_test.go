// Copyright 2024 Block, Inc.

package workload_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/go-test/deep"

	"github.com/square/finch"
	"github.com/square/finch/client"
	"github.com/square/finch/config"
	"github.com/square/finch/data"
	"github.com/square/finch/test/mock"
	"github.com/square/finch/trx"
	"github.com/square/finch/workload"
)

var p = map[string]string{}

func TestGroups_SetupOne(t *testing.T) {
	mockValues := []interface{}{"hello"}
	g := mock.DataGenerator{
		FormatFunc: func() (uint, string) {
			return 1, "%d"
		},
		ValuesFunc: func(_ data.RunCount) []interface{} {
			return mockValues
		},
	}
	gf := mock.DataGeneratorFactory{
		MakeFunc: func(name, dataKey string, params map[string]string) (data.Generator, error) {
			return g, nil
		},
	}
	data.Register("mock", gf)

	// Just one SELECT statement in this trx file
	trxList := []config.Trx{
		{
			Name: "001.sql", // must set; Validate not called
			File: "../test/trx/001.sql",
			Data: map[string]config.Data{
				"id": {
					Generator: "mock",
				},
			},
		},
	}
	scope := data.NewScope()
	set, err := trx.Load(trxList, scope, p)
	if err != nil {
		t.Fatal(err)
	}

	// ----------------------------------------------------------------------
	// Exec group
	//
	// With no work load, all trx should be auto-assigned to 1 exec/client group
	// with clients: 1 and name dml1 (because there's no DDL)
	a := workload.Allocator{
		Stage:     1,
		StageName: "setup",
		TrxSet:    set,
		Workload:  []config.ClientGroup{}, // NO WORKLOAD
	}

	gotGroups, err := a.Groups()
	if err != nil {
		t.Fatal(err)
	}

	execptGroups := [][]int{
		{0},
	}
	if diff := deep.Equal(gotGroups, execptGroups); diff != nil {
		t.Error(diff)
		t.Logf("got: %#v", gotGroups)
	}

	eg := []config.ClientGroup{
		{
			Group:   "dml1",
			Clients: "1",
			Iter:    "",
			Trx:     []string{"001.sql"},
		},
	}
	if diff := deep.Equal(a.Workload, eg); diff != nil {
		t.Error(diff)
		t.Logf("got: %#v", a.Workload)
	}

	// ----------------------------------------------------------------------
	// Clients
	//
	// Given the exec groups from the previous test ^, allocate the Clients.
	// There should only be 1.
	gotClients, err := a.Clients(gotGroups, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(gotClients) != 1 || len(gotClients[0]) == 0 {
		t.Fatalf("expected 1 client, got %#v", gotClients)
	}
	r := finch.RunLevel{
		Stage:         1,
		StageName:     "setup",
		ExecGroup:     1,
		ExecGroupName: "dml1",
		ClientGroup:   1,
		Client:        1,
	}

	expectClients := [][]workload.ClientGroup{
		{ // exec grp 0
			{ // client grp 0
				Runtime: 0,
				Clients: []*client.Client{
					{ // client 0
						RunLevel: r,
						Iter:     0,
						Statements: []*trx.Statement{
							{
								Trx:       "001.sql",
								Query:     "select c from t where id=%d",
								ResultSet: true,
								Inputs:    []string{"@id"},
								Calls:     []byte{0}, // call Generator.Values
							},
						},
						Data: []client.StatementData{
							{
								TrxBoundary: trx.BEGIN | trx.END,
							},
						},
					},
				},
			},
		},
	}
	if diff := deep.Equal(gotClients, expectClients); diff != nil {
		t.Error(diff)
		t.Logf("got: %#v", gotClients)
	}
	if gotClients[0][0].Clients[0].DB == nil {
		t.Errorf("client *sql.DB is nil, expected a *sql.DB to be set")
	}
	if gotClients[0][0].Clients[0].Stats[0] != nil {
		t.Errorf("client *stats.Stats is set, expected nil stats for setup stage")
	}
	if gotClients[0][0].Clients[0].DoneChan != a.DoneChan {
		t.Errorf("client DoneChan != Allocator.DoneChan, expected Allocator to pass DoneChan to Client")
	}

	r = finch.RunLevel{
		Stage:         1,
		StageName:     "setup",
		ExecGroup:     1,
		ExecGroupName: "dml1",
		ClientGroup:   1,
		Client:        1,
		Trx:           1,
		TrxName:       "001.sql",
		Query:         1,
	}

	if len(gotClients[0][0].Clients[0].Data) != 1 || len(gotClients[0][0].Clients[0].Data[0].Inputs) != 1 {
		t.Errorf("got %d data generator, expected 1", len(gotClients[0][0].Clients[0].Data))
	} else {
		f := gotClients[0][0].Clients[0].Data[0].Inputs[0]
		if f == nil {
			t.Fatal("client value func not set")
		}
		gotVals := f(data.RunCount{1, 1, 1, 1})
		if diff := deep.Equal(gotVals, mockValues); diff != nil {
			t.Error(diff)
		}
	}
}

func TestGroups_PartialAlloc(t *testing.T) {
	// The exec group below only sets "clients: 1" and nothing else, so when
	// auto alloc is enabled (Auto: true), all trx should be assigned to the
	// exec group without any explicitly set trx.
	trxList := []config.Trx{
		{
			Name: "001.sql", // must set; Validate not called
			File: "../test/trx/001.sql",
			Data: map[string]config.Data{
				"id": {
					Generator: "auto-inc",
				},
			},
		},
	}
	scope := data.NewScope()
	set, err := trx.Load(trxList, scope, p)
	if err != nil {
		t.Fatal(err)
	}

	a := workload.Allocator{
		Stage:     1,
		StageName: "setup",
		TrxSet:    set,
		Workload: []config.ClientGroup{
			{
				Clients: "1",
				// Trx: []string not set, so this exec group gets all trx
			},
		},
	}

	gotGroups, err := a.Groups()
	if err != nil {
		t.Fatal(err)
	}

	execptGroups := [][]int{{0}}
	if diff := deep.Equal(gotGroups, execptGroups); diff != nil {
		t.Error(diff)
		t.Logf("got: %#v", gotGroups)
	}

	eg := []config.ClientGroup{
		{
			Group:   "dml1",
			Clients: "1",
			Trx:     []string{"001.sql"}, // auto-assigned
		},
	}
	if diff := deep.Equal(a.Workload, eg); diff != nil {
		t.Error(diff)
		t.Logf("got: %#v", a.Workload)
	}
}

func TestGroups_ClientGroups(t *testing.T) {
	stage, err := config.Load([]string{"../test/run/scope/workload_cg_alloc.yaml"}, nil, "dsn", "db")
	if err != nil {
		t.Fatal(err)
	}
	if len(stage) != 1 {
		t.Fatalf("loaded %d stages, expected 1", len(stage))
	}

	if err := os.Chdir("../test/run/scope/"); err != nil {
		t.Fatal(err)
	}

	scope := data.NewScope()
	set, err := trx.Load(stage[0].Trx, scope, p)
	if err != nil {
		t.Fatal(err)
	}
	if set == nil {
		t.Fatal("trx.Load returned nil Set")
	}

	a := workload.Allocator{
		Stage:     1,
		StageName: stage[0].Name,
		TrxSet:    set,
		Workload:  stage[0].Workload,
	}
	groups, err := a.Groups()
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 {
		t.Errorf("got %d exec groups, expected 1", len(groups))
	}
	execGroups, err := a.Clients(groups, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(execGroups) != 1 {
		t.Fatalf("got %d exec groups, expected 1", len(execGroups))
	}
	if len(execGroups[0]) != 2 {
		t.Fatalf("got %d client group in exec groups, expected 2", len(execGroups[0]))
	}
	if len(execGroups[0][0].Clients) != 2 {
		t.Fatalf("got %d clients in e1/g1, expected 2", len(execGroups[0][0].Clients))
	}
	if len(execGroups[0][1].Clients) != 2 {
		t.Fatalf("got %d clients in e1/g2, expected 2", len(execGroups[0][1].Clients))
	}

	for i := 0; i < 2; i++ {
		got := execGroups[0][0].Clients[i].RunLevel.ClientId()
		expect := fmt.Sprintf("1(test)/e1(__A__)/g1/c%d", i+1)
		if got != expect {
			t.Errorf("cg 1/c%d: got %s, expected %s", i, got, expect)
		}
	}
	for i := 0; i < 2; i++ {
		got := execGroups[0][1].Clients[i].RunLevel.ClientId()
		expect := fmt.Sprintf("1(test)/e1(__A__)/g2/c%d", i+1)
		if got != expect {
			t.Errorf("cg 2/c%d: got %s, expected %s", i, got, expect)
		}
	}
}
