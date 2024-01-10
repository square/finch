package boot_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"

	"github.com/go-test/deep"

	"github.com/square/finch/boot"
	"github.com/square/finch/test"
)

var (
	dsn string
	db  *sql.DB
)

func setup(t *testing.T) {
	var err error
	var queries []string
	if dsn == "" || db == nil {
		dsn, db, err = test.Connection()
		if err != nil {
			t.Fatal(err)
		}
		queries = []string{
			"CREATE DATABASE IF NOT EXISTS finch",
			"USE finch",
			"DROP TABLE IF EXISTS scopetest",
			"CREATE TABLE scopetest (id int auto_increment primary key not null, eg int, cg int, c int, iter int, trx int, s int, a int, r int)",
		}
	} else {
		queries = []string{
			"USE finch",
			"TRUNCATE TABLE scopetest",
		}
	}
	if err := test.Exec(db, queries); err != nil {
		t.Fatal(err)
	}
}

func results() ([][]int, error) {
	rows, err := db.QueryContext(context.Background(), "select eg, cg, c, iter, trx, s, a, r from finch.scopetest order by eg, cg, c, iter, trx, s")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	res := [][]int{}
	for rows.Next() {
		cols := make([]int, 8)
		ptr := make([]interface{}, len(cols))
		for i := range ptr {
			ptr[i] = &cols[i]
		}
		if err = rows.Scan(ptr...); err != nil {
			return nil, err
		}
		res = append(res, cols)
	}
	return res, nil
}

const (
	EG int = iota
	CG
	C
	ITER
	TRX
	S
	A
	R
)

func colVals(col int, rows [][]int) []int {
	vals := make([]int, len(rows))
	for i := range rows {
		vals[i] = rows[i][col]
	}
	return vals
}

func hasPattern(p string, vals []int) (bool, string) {
	az := 96 // 97=a
	seen := map[int]bool{}
	q := ""
	for _, v := range vals {
		if !seen[v] {
			az += 1 // a, c, ...
			seen[v] = true
		}
		q += fmt.Sprintf("%c", az)
	}
	return p == q, q
}

func run(t *testing.T, file string) ([][]int, error) {
	setup(t)
	defer os.Chdir(cwd) // Finch will cd to stage file dir
	env := boot.Env{
		Args: []string{
			"./finch", // fake like it was run from cmd line (required)
			"-D", "finch",
			"--dsn", dsn,
			"../test/run/scope/" + file,
		},
	}
	err := boot.Up(env)
	if err != nil {
		return nil, err
	}
	rows, err := results()
	if err != nil {
		return nil, err
	}
	if len(rows) != 12 {
		return nil, fmt.Errorf("got %d rows, expected 12", len(rows))
	}
	return rows, nil
}

// --------------------------------------------------------------------------

func TestScope_Statement(t *testing.T) {
	if test.Build {
		t.Skip("GitHub Actions build")
	}
	rows, err := run(t, "statement.yaml")
	if err != nil {
		t.Fatal(err)
	}

	/*
		+------+------+------+------+------+------+------+--------+
		| eg   | cg   | c    | iter | trx  | s    | a    | r      |
		+------+------+------+------+------+------+------+--------+
		|    1 |    1 |    1 |    1 |    1 |    1 |    1 | 550747 |
		|    1 |    1 |    1 |    1 |    1 |    2 |    1 | 736643 |
		|    1 |    1 |    1 |    1 |    2 |    3 |    1 | 421079 |
		|    1 |    1 |    1 |    2 |    3 |    4 |    2 | 662756 |
		|    1 |    1 |    1 |    2 |    3 |    5 |    2 | 135600 |
		|    1 |    1 |    1 |    2 |    4 |    6 |    2 | 331290 |
		|    1 |    2 |    1 |    1 |    1 |    1 |    1 | 952510 |
		|    1 |    2 |    1 |    1 |    1 |    2 |    1 | 163171 |
		|    1 |    2 |    1 |    1 |    2 |    3 |    1 | 979482 |
		|    2 |    1 |    1 |    1 |    1 |    1 |    1 | 351387 |
		|    2 |    1 |    1 |    1 |    1 |    2 |    1 | 702906 |
		|    2 |    1 |    1 |    1 |    2 |    3 |    1 | 198349 |
		+------+------+------+------+------+------+------+--------+
	*/

	// A (auto-inc) values equal the above ^ example
	got := colVals(A, rows)
	expect := []int{1, 1, 1, 2, 2, 2, 1, 1, 1, 1, 1, 1}
	if diff := deep.Equal(got, expect); diff != nil {
		t.Errorf("wrong auto-inc values: %v", diff)
	}

	got = colVals(R, rows)
	seen := map[int]bool{}
	for _, i := range got {
		if seen[i] {
			t.Errorf("same random value with statement scope, expected all random values (or 1 in a 1,000,000 chance @r generated same random value): %d", i)
		}
	}
}

func TestScope_Trx(t *testing.T) {
	if test.Build {
		t.Skip("GitHub Actions build")
	}

	rows, err := run(t, "trx.yaml")
	if err != nil {
		t.Fatal(err)
	}

	/*
		+------+------+------+------+------+------+------+--------+
		| eg   | cg   | c    | iter | trx  | s    | a    | r      |
		+------+------+------+------+------+------+------+--------+
		|    1 |    1 |    1 |    1 |    1 |    1 |    1 | 566273 | a
		|    1 |    1 |    1 |    1 |    1 |    2 |    1 | 566273 | a
		|    1 |    1 |    1 |    1 |    2 |    3 |    1 | 995357 | b
		|    1 |    1 |    1 |    2 |    3 |    4 |    2 | 919473 | c
		|    1 |    1 |    1 |    2 |    3 |    5 |    2 | 919473 | c
		|    1 |    1 |    1 |    2 |    4 |    6 |    2 | 509510 | d
		|    1 |    2 |    1 |    1 |    1 |    1 |    1 | 151654 | e
		|    1 |    2 |    1 |    1 |    1 |    2 |    1 | 151654 | e
		|    1 |    2 |    1 |    1 |    2 |    3 |    1 | 418694 | f
		|    2 |    1 |    1 |    1 |    1 |    1 |    1 | 495910 | g
		|    2 |    1 |    1 |    1 |    1 |    2 |    1 | 495910 | g
		|    2 |    1 |    1 |    1 |    2 |    3 |    1 | 841785 | h
		+------+------+------+------+------+------+------+--------+
	*/

	// A (auto-inc) values equal the above ^ example, which is same as statement
	// scope, so looking at r for trx scope is better (below)
	got := colVals(A, rows)
	expect := []int{1, 1, 1, 2, 2, 2, 1, 1, 1, 1, 1, 1}
	if diff := deep.Equal(got, expect); diff != nil {
		t.Errorf("wrong auto-inc values: %v", diff)
	}

	// Check that random numbers repeat as expected by reducing each unique
	// value to a, b, c, etc. then comparing the patterns
	rVals := colVals(R, rows)
	p := "aabccdeefggh"
	if ok, q := hasPattern(p, rVals); !ok {
		t.Errorf("random vals %v have pattern %s, expected %s ", rVals, q, p)
	}
}

func TestScope_Iter(t *testing.T) {
	if test.Build {
		t.Skip("GitHub Actions build")
	}

	rows, err := run(t, "iter.yaml")
	if err != nil {
		t.Fatal(err)
	}

	/*
		+------+------+------+------+------+------+------+--------+
		| eg   | cg   | c    | iter | trx  | s    | a    | r      |
		+------+------+------+------+------+------+------+--------+
		|    1 |    1 |    1 |    1 |    1 |    1 |    1 | 304611 | a
		|    1 |    1 |    1 |    1 |    1 |    2 |    1 | 304611 | a
		|    1 |    1 |    1 |    1 |    2 |    3 |    1 | 304611 | a
		|    1 |    1 |    1 |    2 |    3 |    4 |    2 | 945546 | b
		|    1 |    1 |    1 |    2 |    3 |    5 |    2 | 945546 | b
		|    1 |    1 |    1 |    2 |    4 |    6 |    2 | 945546 | b
		|    1 |    2 |    1 |    1 |    1 |    1 |    1 | 777638 | c
		|    1 |    2 |    1 |    1 |    1 |    2 |    1 | 777638 | c
		|    1 |    2 |    1 |    1 |    2 |    3 |    1 | 777638 | c
		|    2 |    1 |    1 |    1 |    1 |    1 |    1 | 553149 | d
		|    2 |    1 |    1 |    1 |    1 |    2 |    1 | 553149 | d
		|    2 |    1 |    1 |    1 |    2 |    3 |    1 | 553149 | d
		+------+------+------+------+------+------+------+--------+
	*/

	// A (auto-inc) values equal the above ^ example, which is same as statement
	// scope, so looking at r for trx scope is better (below)
	got := colVals(A, rows)
	expect := []int{1, 1, 1, 2, 2, 2, 1, 1, 1, 1, 1, 1}
	if diff := deep.Equal(got, expect); diff != nil {
		t.Errorf("wrong auto-inc values: %v", diff)
	}

	// See previous test
	rVals := colVals(R, rows)
	p := "aaabbbcccddd"
	if ok, q := hasPattern(p, rVals); !ok {
		t.Errorf("random vals %v have pattern %s, expected %s ", rVals, q, p)
	}
}

func TestScope_Client(t *testing.T) {
	if test.Build {
		t.Skip("GitHub Actions build")
	}

	rows, err := run(t, "client.yaml")
	if err != nil {
		t.Fatal(err)
	}

	/*
		+------+------+------+------+------+------+------+--------+
		| eg   | cg   | c    | iter | trx  | s    | a    | r      |
		+------+------+------+------+------+------+------+--------+
		|    1 |    1 |    1 |    1 |    1 |    1 |    1 | 782998 | a
		|    1 |    1 |    1 |    1 |    1 |    2 |    1 | 782998 | a
		|    1 |    1 |    1 |    1 |    2 |    3 |    1 | 782998 | a
		|    1 |    1 |    1 |    2 |    3 |    4 |    1 | 782998 | a
		|    1 |    1 |    1 |    2 |    3 |    5 |    1 | 782998 | a
		|    1 |    1 |    1 |    2 |    4 |    6 |    1 | 782998 | a
		|    1 |    2 |    1 |    1 |    1 |    1 |    1 |  88789 | b
		|    1 |    2 |    1 |    1 |    1 |    2 |    1 |  88789 | b
		|    1 |    2 |    1 |    1 |    2 |    3 |    1 |  88789 | b
		|    2 |    1 |    1 |    1 |    1 |    1 |    1 |  93318 | c
		|    2 |    1 |    1 |    1 |    1 |    2 |    1 |  93318 | c
		|    2 |    1 |    1 |    1 |    2 |    3 |    1 |  93318 | c
		+------+------+------+------+------+------+------+--------+
	*/

	got := colVals(A, rows)
	expect := []int{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}
	if diff := deep.Equal(got, expect); diff != nil {
		t.Errorf("wrong auto-inc values: %v", diff)
	}

	// See previous test
	rVals := colVals(R, rows)
	p := "aaaaaabbbccc"
	if ok, q := hasPattern(p, rVals); !ok {
		t.Errorf("random vals %v have pattern %s, expected %s ", rVals, q, p)
	}
}
