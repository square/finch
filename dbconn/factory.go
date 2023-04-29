// Copyright 2023 Block, Inc.

// Package dbconn provides a Factory that makes *sql.DB connections to MySQL.
package dbconn

import (
	"database/sql"
	"fmt"
	"io/fs"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/go-sql-driver/mysql"

	"github.com/square/finch"
	"github.com/square/finch/aws"
	"github.com/square/finch/config"
)

// rdsAddr matches Amazon RDS hostnames with optional :port suffix.
// It's used to automatically load the Amazon RDS CA and enable TLS,
// unless config.aws.disable-auto-tls is true.
var rdsAddr = regexp.MustCompile(`rds\.amazonaws\.com(:\d+)?$`)

// portSuffix matches optional :port suffix on addresses. It's used to
// strip the port suffix before passing the hostname to LoadTLS.
var portSuffix = regexp.MustCompile(`:\d+$`)

var f = &factory{}

type factory struct {
	cfg      config.MySQL
	modifyDB func(*sql.DB, string)
	// --
	loaded bool
	dsn    string
}

func SetFactory(cfg config.MySQL, modifyDB func(*sql.DB, string)) {
	f.cfg = cfg
	f.modifyDB = modifyDB
}

func Make() (*sql.DB, string, error) {
	if !f.loaded {
		if err := f.setDSN(); err != nil {
			return nil, "", err
		}
		f.loaded = true
		log.Printf("%s\n", RedactedDSN(f.dsn))
	}

	db, err := sql.Open("mysql", f.dsn)
	if err != nil {
		return nil, "", err
	}

	db.SetMaxOpenConns(1)

	// Let user-provided plugin set/change DB
	if f.modifyDB != nil {
		f.modifyDB(db, f.dsn)
	}

	return db, RedactedDSN(f.dsn), nil
}

func (f *factory) setDSN() error {
	if f.cfg.DSN != "" {
		f.dsn = f.cfg.DSN
		return nil
	}

	// ----------------------------------------------------------------------
	// my.cnf

	// Set values in cfg finch.ConfigMonitor from values in my.cnf. This does
	// not overwrite any values in cfg already set. For exmaple, if username
	// is specified in both, the default my.cnf username is ignored and the
	// explicit cfg.Username is kept/used.
	if f.cfg.MyCnf != "" {
		finch.Debug("read mycnf %s", f.cfg.MyCnf)
		def, err := ParseMyCnf(f.cfg.MyCnf)
		if err != nil {
			return err
		}
		f.cfg.With(def)
	}

	// ----------------------------------------------------------------------
	// TCP or Unix socket

	net := ""
	addr := ""
	if f.cfg.Socket != "" {
		net = "unix"
		addr = f.cfg.Socket
	} else {
		net = "tcp"
		addr = f.cfg.Hostname
	}

	// ----------------------------------------------------------------------
	// Load TLS

	params := []string{"parseTime=true"}

	// Go says "either ServerName or InsecureSkipVerify must be specified".
	// This is a pathological case: socket and TLS but no hostname to verify
	// and user didn't explicitly set skip-verify=true. So we set this latter
	// automatically because Go will certainly error if we don't.
	if net == "unix" && f.cfg.TLS.Set() && f.cfg.Hostname == "" && !config.True(f.cfg.TLS.SkipVerify) {
		b := true
		f.cfg.TLS.SkipVerify = &b
		finch.Debug("auto-enabled skip-verify on socket with TLS but no hostname")
	}

	// Load and register TLS, if any
	tlsConfig, err := f.cfg.TLS.LoadTLS(portSuffix.ReplaceAllString(f.cfg.Hostname, ""))
	if err != nil {
		return err
	}
	if tlsConfig != nil {
		mysql.RegisterTLSConfig("benchmark", tlsConfig)
		params = append(params, "tls=benchmark")
		finch.Debug("TLS enabled")
	}

	// Use built-in Amazon RDS CA
	if rdsAddr.MatchString(addr) && !config.True(f.cfg.DisableAutoTLS) && tlsConfig == nil {
		finch.Debug("auto AWS TLS: hostname has suffix .rds.amazonaws.com")
		aws.RegisterRDSCA() // safe to call multiple times
		params = append(params, "tls=rds")
	}

	// ----------------------------------------------------------------------
	// Credentials (user:pass)

	var password = f.cfg.Password
	if f.cfg.PasswordFile != "" {
		bytes, err := ioutil.ReadFile(f.cfg.PasswordFile)
		if err != nil {
			return err
		}
		password = string(bytes)
	}

	cred := f.cfg.Username
	if password != "" {
		cred += ":" + password
	}

	// ----------------------------------------------------------------------
	// Set DSN

	f.dsn = fmt.Sprintf("%s@%s(%s)/", cred, net, addr)
	if len(params) > 0 {
		f.dsn += "?" + strings.Join(params, "&")
	}

	return nil
}

const (
	default_mysql_socket  = "/tmp/mysql.sock"
	default_distro_socket = "/var/lib/mysql/mysql.sock"
)

func Sockets() []string {
	sockets := []string{}
	seen := map[string]bool{}
	for _, socket := range strings.Split(socketList(), "\n") {
		socket = strings.TrimSpace(socket)
		if socket == "" {
			continue
		}
		if seen[socket] {
			continue
		}
		seen[socket] = true
		if !isSocket(socket) {
			continue
		}
		sockets = append(sockets, socket)
	}

	if len(sockets) == 0 {
		finch.Debug("no sockets, using defaults")
		if isSocket(default_mysql_socket) {
			sockets = append(sockets, default_mysql_socket)
		}
		if isSocket(default_distro_socket) {
			sockets = append(sockets, default_distro_socket)
		}
	}

	finch.Debug("sockets: %v", sockets)
	return sockets
}

func socketList() string {
	cmd := exec.Command("sh", "-c", "netstat -f unix | grep mysql | grep -v mysqlx | awk '{print $NF}'")
	output, err := cmd.Output()
	if err != nil {
		finch.Debug(err.Error())
	}
	return string(output)
}

func isSocket(file string) bool {
	fi, err := os.Stat(file)
	if err != nil {
		return false
	}
	return fi.Mode()&fs.ModeSocket != 0
}

func RedactedDSN(dsn string) string {
	redactedPassword, err := mysql.ParseDSN(dsn)
	if err != nil { // ok to ignore
		finch.Debug("mysql.ParseDSN error: %s", err)
	}
	redactedPassword.Passwd = "..."
	return redactedPassword.FormatDSN()

}
