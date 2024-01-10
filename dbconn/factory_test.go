// Copyright 2024 Block, Inc.

package dbconn_test

import (
	"strings"
	"testing"

	"github.com/go-sql-driver/mysql"

	"github.com/square/finch/config"
	"github.com/square/finch/dbconn"
	"github.com/square/finch/test"
)

func TestMake_DSN(t *testing.T) {
	if test.Build {
		t.Skip("GitHub Actions build")
	}

	// Test connection
	dsn, db, err := test.Connection()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Make factory-made connect with test DSN
	cfg := config.MySQL{
		DSN: dsn,
	}
	dbconn.SetConfig(cfg)

	fdb, fdsn, err := dbconn.Make()
	if err != nil {
		t.Error(err)
	}

	if fdb == nil {
		t.Fatal("got nil *sql.DB")
	}
	defer fdb.Close()

	if fdsn != strings.Replace(dsn, "test", "...", 1) { // factory DSN redacts password to "..."
		t.Errorf("got dsn '%s', expected '%s'", fdsn, dsn)
	}

}

func TestMake_Config(t *testing.T) {
	if test.Build {
		t.Skip("GitHub Actions build")
	}

	// Test connection
	dsn, db, err := test.Connection()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Parse test DSN so we can map its parts to a Finch config.MySQL
	my, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}

	// Make factory-made connect with test DSN
	cfg := config.MySQL{
		Hostname: my.Addr, // has ":test.MySQLPort" suffix
		Password: my.Passwd,
		Username: my.User,
	}
	dbconn.SetConfig(cfg)

	fdb, _, err := dbconn.Make()
	if err != nil {
		t.Error(err)
	}
	if fdb == nil {
		t.Fatal("got nil *sql.DB")
	}
	defer fdb.Close()

	got, err := test.OneRow(fdb, "SELECT @@version")
	if err != nil {
		t.Error(err)
	}
	if got != "8.0.34" {
		t.Errorf("SELECT @@version: got %s, expected 8.0.34", got)
	}
}
