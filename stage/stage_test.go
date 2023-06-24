package stage

import (
	"context"
	"testing"

	"github.com/square/finch/config"
	"github.com/square/finch/data"
	"github.com/square/finch/test"
)

func TestPreapre_NoWorkload(t *testing.T) {
	if test.Build {
		t.Skip("GitHub Actions build")
	}

	dsn, db, err := test.Connection()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	cfg := config.Stage{
		Name: "test",
		Trx: []config.Trx{
			{
				Name: "001",
				File: "../test/trx/001.sql",
				Data: map[string]config.Data{
					"id": {
						Generator: "int",
					},
				},
			},
		},
		MySQL: config.MySQL{
			DSN: dsn,
		},
		Workload: []config.ClientGroup{}, // no workload
	}
	gds := data.NewScope() // global data scope

	s := New(cfg, gds, nil)
	err = s.Prepare(context.Background())
	if err != nil {
		t.Error(err)
	}

	// There should be 1 exec group, 1 client group, and 1 client
	if len(s.execGroups) != 1 {
		t.Fatalf("got %d exec group, expected 1", len(s.execGroups))
	}
	if len(s.execGroups[0]) != 1 {
		t.Fatalf("got %d clients, expected 1", len(s.execGroups[0]))
	}
}
