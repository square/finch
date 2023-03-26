// Copyright 2023 Block, Inc.

package workload_test

import (
	"testing"

	"github.com/go-test/deep"

	"github.com/square/finch"
	"github.com/square/finch/client"
	"github.com/square/finch/config"
	"github.com/square/finch/data"
	"github.com/square/finch/trx"
	"github.com/square/finch/workload"
)

func TestGroups_SetupOne(t *testing.T) {
	trxList := []config.Trx{
		{
			Name: "001.sql", // must set; Validate not called
			File: "../test/trx/001.sql",
			Data: map[string]config.Data{
				"id": {
					Generator: "uint64-counter",
				},
			},
		},
	}
	scope := data.NewScope()
	set, err := trx.Load(trxList, scope)
	if err != nil {
		t.Fatal(err)
	}

	a := workload.Allocator{
		Stage:      "setup",
		TrxSet:     set,
		ExecGroups: []config.ExecGroup{},
		ExecMode:   finch.EXEC_SEQUENTIAL, // must set; Validate not called
		Auto:       true,
		DoneChan:   make(chan *client.Client, 1),
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

	eg := []config.ExecGroup{
		{
			Name:    "e1",
			Clients: 1,
			Iter:    1,
			Trx:     []string{"001.sql"},
		},
	}
	if diff := deep.Equal(a.ExecGroups, eg); diff != nil {
		t.Error(diff)
		t.Logf("got: %#v", a.ExecGroups)
	}

	a = workload.Allocator{
		Stage:      "setup",
		TrxSet:     set,
		ExecGroups: []config.ExecGroup{},
		ExecMode:   finch.EXEC_CONCURRENT, // must set; Validate not called
		Auto:       true,
	}
	gotGroups, err = a.Groups()
	if err != nil {
		t.Fatal(err)
	}
	if diff := deep.Equal(gotGroups, execptGroups); diff != nil {
		t.Error(diff)
		t.Logf("got: %#v", gotGroups)
	}

	// Clients
	gotClients, err := a.Clients(gotGroups)
	if err != nil {
		t.Fatal(err)
	}
	if len(gotClients) != 1 || len(gotClients[0]) == 0 {
		t.Fatalf("expected 1 client, got %#v", gotClients)
	}
	r := finch.RunLevel{
		Stage:         "setup",
		ExecGroup:     1,
		ExecGroupName: "e1",
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
						Iter:     1,
						Statements: []*trx.Statement{
							{
								Trx:       "001.sql",
								Query:     "select c from t where id=%d",
								ResultSet: true,
								Inputs:    []string{"@id"},
							},
						},
						Data: []trx.Data{
							{
								TrxBoundary: finch.TRX_BEGIN | finch.TRX_END,
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
		Stage:         "setup",
		ExecGroup:     1,
		ExecGroupName: "e1",
		ClientGroup:   1,
		Client:        1,
		Trx:           1,
		TrxName:       "001.sql",
		Query:         1,
	}

	if len(gotClients[0][0].Clients[0].Data) != 1 || len(gotClients[0][0].Clients[0].Data[0].Inputs) != 1 {
		t.Errorf("got %d data generator, expected 1", len(gotClients[0][0].Clients[0].Data))
	} else {
		gotId := gotClients[0][0].Clients[0].Data[0].Inputs[0].Id()
		expectId := data.Id{
			RunLevel: r,
			Type:     "uint64-counter",
			DataKey:  "@id",
			Scope:    "statement",
			CopyNo:   1,
		}
		if diff := deep.Equal(gotId, expectId); diff != nil {
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
					Generator: "uint64-counter",
				},
			},
		},
	}
	scope := data.NewScope()
	set, err := trx.Load(trxList, scope)
	if err != nil {
		t.Fatal(err)
	}

	a := workload.Allocator{
		Stage:  "setup",
		TrxSet: set,
		ExecGroups: []config.ExecGroup{
			{
				Clients: 1,
				// Trx: []string not set, so this exec group gets all trx
			},
		},
		ExecMode: finch.EXEC_SEQUENTIAL, // must set; Validate not called
		Auto:     true,
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

	eg := []config.ExecGroup{
		{
			Name:    "e1",
			Clients: 1,
			Trx:     []string{"001.sql"}, // auto-assigned
		},
	}
	if diff := deep.Equal(a.ExecGroups, eg); diff != nil {
		t.Error(diff)
		t.Logf("got: %#v", a.ExecGroups)
	}
}
