package test

import (
	"database/sql"
	"fmt"
	"os"
)

// Build is true when running in GitHub Actions. When true, database tests are
// skipped because currently we don't run MySQL in GitHub Acitons, but other tests
// and the Go build still run.
var Build = os.Getenv("GITHUB_ACTION") != ""

var MySQLPort = "33800" // test/docker/docker-compose.yaml

func Connection() (string, *sql.DB, error) {
	dsn := fmt.Sprintf(
		"%s:%s@tcp(%s:%s)/?parseTime=true",
		"root",
		"test",
		"127.0.0.1",
		MySQLPort,
	)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return "", nil, err
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return "", nil, err
	}
	return dsn, db, nil
}
