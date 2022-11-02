// Copyright 2022 Block, Inc.

package workload_test

import (
	"testing"

	"github.com/square/finch/config"
	"github.com/square/finch/workload"
)

func TestLoad(t *testing.T) {
	trx := config.Trx{
		Name: "test",
		File: "../test/statements/s1.sql",
		Data: map[string]config.Data{
			"id": {
				Generator: "int-not-null",
			},
		},
	}

	got, err := workload.Load(trx)
	if err != nil {
		t.Fatal(err)
	}
	t.Errorf("%+v", got)
}

func TestLoad2(t *testing.T) {
	trx := config.Trx{
		Name: "test",
		File: "../test/sb/insert-rows.sql",
		Data: map[string]config.Data{
			"id": {
				Generator: "int-not-null",
			},
			"c": {
				Generator: "int-not-null",
			},
			"k": {
				Generator: "int-not-null",
			},
			"pad": {
				Generator: "int-not-null",
			},
		},
	}

	got, err := workload.Load(trx)
	if err != nil {
		t.Fatal(err)
	}
	t.Errorf("%+v", got)
}
