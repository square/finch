// Copyright 2024 Block, Inc.

// Package dbconn provides a Factory that makes *sql.DB connections to MySQL.
package dbconn

import (
	"database/sql"
	"fmt"
	"io/fs"
	"io/ioutil"
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
	cfg config.MySQL
	dsn string
}

func SetConfig(cfg config.MySQL) {
	f.cfg = cfg
	f.dsn = ""
}

func Make() (*sql.DB, string, error) {
	// Parse MySQL params and set DSN on first call. There's only 1 DSN for
	// all clients, so this only needs to be done once.
	if f.dsn == "" {
		if err := f.setDSN(); err != nil {
			return nil, "", err
		}
	}
	finch.Debug("dsn: %s", RedactedDSN(f.dsn))

	// Make new sql.DB (conn pool) for each client group; see the call to
	// this func in workload/workload.go.
	db, err := sql.Open("mysql", f.dsn)
	if err != nil {
		return nil, "", err
	}
	return db, RedactedDSN(f.dsn), nil
}

func (f *factory) setDSN() error {
	// --dsn or mysql.dsn (in that order) overrides all
	if f.cfg.DSN != "" {
		f.dsn = f.cfg.DSN
		return nil
	}

	// ----------------------------------------------------------------------
	// my.cnf

	// Set values in cfg from values in my.cnf. This does not overwrite any
	// values in cfg already set. For exmaple, if username is specified in both,
	// the default my.cnf username is ignored and the explicit cfg.Username is
	// kept/used.
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
		if f.cfg.Hostname == "" {
			f.cfg.Hostname = "127.0.0.1"
		}
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

	if f.cfg.Username == "" {
		f.cfg.Username = "finch" // default username
		finch.Debug("using default MySQL username")
		if f.cfg.Password == "" && password == "" {
			finch.Debug("using default MySQL password")
			password = "amazing"
		}
	}
	cred := f.cfg.Username
	if password != "" {
		cred += ":" + password
	}

	// ----------------------------------------------------------------------
	// Set DSN

	f.dsn = fmt.Sprintf("%s@%s(%s)/%s", cred, net, addr, f.cfg.Db)
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

// Suppress error messages from Go MySQL driver like "[mysql] 2023/07/27 15:59:30 packets.go:37: unexpected EOF"
type null struct{}

func (n null) Print(v ...interface{}) {}

func init() {
	mysql.SetLogger(null{})
}
