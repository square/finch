package compute_test

import (
	"context"
	"os"
	"testing"

	"github.com/square/finch/compute"
	"github.com/square/finch/config"
	"github.com/square/finch/test"
)

func TestServer_Run1Stage(t *testing.T) {
	if test.Build {
		t.Skip("GitHub Actions build")
	}

	dsn, db, err := test.Connection()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	os.Setenv("PORT", test.MySQLPort)
	stages, err := config.Load(
		[]string{"../test/run/select-1/test.yaml"},
		[]string{},
		dsn,
		"", // default db
	)
	if err != nil {
		t.Fatal(err)
	}

	s := compute.NewServer("local", "", false)

	err = s.Run(context.Background(), stages)
	if err != nil {
		t.Error(err)
	}
}
