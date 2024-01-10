// Copyright 2024 Block, Inc.

package client_test

import (
	"context"
	"testing"
	"time"

	"github.com/go-test/deep"

	"github.com/square/finch"
	"github.com/square/finch/client"
	"github.com/square/finch/data"
	"github.com/square/finch/stats"
	"github.com/square/finch/test"
	"github.com/square/finch/trx"
)

var rl = finch.RunLevel{
	Stage:       1,
	ExecGroup:   1,
	ClientGroup: 1,
	Client:      1,
}

func TestClient_SELECT_1(t *testing.T) {
	if test.Build {
		t.Skip("GitHub Actions build")
	}

	_, db, err := test.Connection()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	doneChan := make(chan *client.Client, 1)

	c := &client.Client{
		DB:       db,
		RunLevel: rl,
		DoneChan: doneChan,
		Statements: []*trx.Statement{
			{
				Query:     "SELECT 1",
				ResultSet: true,
			},
		},
		Data: []client.StatementData{
			{
				TrxBoundary: trx.BEGIN | trx.END,
			},
		},
		Stats: []*stats.Trx{nil},
		// --
		Iter: 1, // need some runtime limit
	}

	err = c.Init()
	if err != nil {
		t.Fatal(err)
	}

	c.Run(context.Background())

	timeout := time.After(2 * time.Second)
	var ret *client.Client
	select {
	case ret = <-doneChan:
	case <-timeout:
		t.Fatal("Client timeout after 2s")
	}

	if ret != c {
		t.Errorf("returned *Client != run *Client")
	}

	if ret.Error.Err != nil {
		t.Errorf("Client error: %v", ret.Error.Err)
	}
}

func TestClient_Write(t *testing.T) {
	if test.Build {
		t.Skip("GitHub Actions build")
	}

	_, db, err := test.Connection()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	queries := []string{
		"CREATE DATABASE IF NOT EXISTS finch",
		"USE finch",
		"DROP TABLE IF EXISTS writetest",
		"CREATE TABLE writetest (i int auto_increment primary key not null, d int)",
	}
	for _, q := range queries {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("%s: %s", q, err)
		}
	}

	// Returns value for input for @d to INSERT
	vals := []interface{}{int64(1)}
	valueFunc := func(_ data.RunCount) []interface{} {
		return vals
	}
	// Receives last insertt ID (1 as well since it's first row)
	col := data.NewColumn(nil)

	doneChan := make(chan *client.Client, 1)

	c := &client.Client{
		DB:       db,
		RunLevel: rl,
		Iter:     1,
		DoneChan: doneChan,
		Statements: []*trx.Statement{
			{
				Query:  "INSERT INTO writetest VALUES (NULL, %d)",
				Write:  true,
				Inputs: []string{"@d"},
			},
		},
		Data: []client.StatementData{
			{
				TrxBoundary: trx.BEGIN | trx.END,
				Inputs:      []data.ValueFunc{valueFunc},
				InsertId:    col,
			},
		},
		Stats: []*stats.Trx{nil},
	}

	err = c.Init()
	if err != nil {
		t.Fatal(err)
	}

	c.Run(context.Background())

	timeout := time.After(2 * time.Second)
	var ret *client.Client
	select {
	case ret = <-doneChan:
	case <-timeout:
		t.Fatal("Client timeout after 2s")
	}

	if ret != c {
		t.Errorf("returned *Client != run *Client")
	}

	if ret.Error.Err != nil {
		t.Errorf("Client error: %v", ret.Error.Err)
	}

	// Auot inc insert id == 1
	got := col.Values(data.RunCount{})
	if diff := deep.Equal(got, vals); diff != nil {
		t.Error(diff)
	}
}
