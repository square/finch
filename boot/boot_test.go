package boot_test

import (
	"os"
	"testing"
	"time"

	"github.com/square/finch/boot"
	"github.com/square/finch/test"
)

var cwd, _ = os.Getwd()

func TestBootAndRuntime(t *testing.T) {
	if test.Build {
		t.Skip("GitHub Actions build")
	}

	defer os.Chdir(cwd)

	dsn, db, err := test.Connection()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	env := boot.Env{
		Args: []string{
			"./finch", // fake like it was run from cmd line (required)
			"--dsn", dsn,
			"../test/run/select-1/test.yaml",
		},
	}

	// Benchmark just runs SELECT 1 for 1s
	t0 := time.Now()
	err = boot.Up(env)
	if err != nil {
		t.Error(err)
	}

	d := time.Now().Sub(t0)
	if d.Seconds() < 0.8 || d.Seconds() > 1.2 {
		t.Errorf("ran for %.1f seconds, epxected 1.0 +/- 0.2", d.Seconds())
	}
}

func TestColumns(t *testing.T) {
	if test.Build {
		t.Skip("GitHub Actions build")
	}

	defer os.Chdir(cwd)

	dsn, db, err := test.Connection()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	env := boot.Env{
		Args: []string{
			"./finch", // fake like it was run from cmd line (required)
			"--dsn", dsn,
			"../test/run/columns/test.yaml",
		},
	}

	err = boot.Up(env)
	if err != nil {
		t.Error(err)
	}

	/*
		finch> select * from coltest1; select * from coltest2; select * from coltest3;
		+---+---+
		| a | b |
		+---+---+
		| 1 | 0 |
		+---+---+

		+---+------+---+
		| x | y    | z |
		+---+------+---+
		| 1 | 0x75 | 0 |
		+---+------+---+

		+---+------+
		| x | y    |
		+---+------+
		| 1 | 0x75 |
		+---+------+

		The first statement uses save-insert-id, which should get value 1 -> @a.
		The second statement inserts @d and "0x75" into coltest2, to verify that save-insert-id works.
		The third statements selects from coltest2 using save-columns: @x, @y, _.
		The fourth statement inserts @x and @y into coltest3 to ensure save-columns works.
		The "0x75" is a trick to ensure that quote-value: yes on @y works: if it does then we
		get "0x75", but if it fails then MySQL changes 0x75 -> "u" and the t3 test fails.
		All this also tests that columns default to trx scope, else none of it works.
	*/

	t1, err := test.OneRow(db, "SELECT CONCAT_WS(',',a,b) FROM finch.coltest1")
	if err != nil {
		t.Error(err)
	}
	if t1 != "1,0" {
		t.Errorf("coltest1 row = '%s', expected '1,0'", t1)
	}

	t2, err := test.OneRow(db, "SELECT CONCAT_WS(',',x,y,z) FROM finch.coltest2")
	if err != nil {
		t.Error(err)
	}
	if t2 != "1,0x75,0" {
		t.Errorf("coltest1 row = '%s', expected '1,0x75,0'", t2)
	}

	t3, err := test.OneRow(db, "SELECT CONCAT_WS(',',x,y) FROM finch.coltest3")
	if err != nil {
		t.Error(err)
	}
	if t3 != "1,0x75" {
		t.Errorf("coltest1 row = '%s', expected '1,0x75'", t3)
	}
}
