// Copyright 2024 Block, Inc.

package finch

import (
	"database/sql"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	VERSION = "1.0.0"

	DEFAULT_SERVER_PORT = "33075"

	COPY_NUMBER = `/*!copy-number*/`

	NOOP_COLUMN = "_"

	ROWS = 100000
)

type RunLevel struct {
	Stage         uint
	StageName     string
	ExecGroup     uint
	ExecGroupName string
	ClientGroup   uint
	Client        uint
	Trx           uint
	TrxName       string
	Query         uint64
}

func (s RunLevel) String() string {
	return fmt.Sprintf("%d(%s)/e%d(%s)/g%d/c%d/t%d(%s)/q%d",
		s.Stage, s.StageName, s.ExecGroup, s.ExecGroupName, s.ClientGroup, s.Client, s.Trx, s.TrxName, s.Query)
}

func (s RunLevel) ClientId() string {
	return fmt.Sprintf("%d(%s)/e%d(%s)/g%d/c%d",
		s.Stage, s.StageName, s.ExecGroup, s.ExecGroupName, s.ClientGroup, s.Client)
}

const (
	SCOPE_GLOBAL       = "global"
	SCOPE_STAGE        = "stage"
	SCOPE_WORKLOAD     = "workload"
	SCOPE_EXEC_GROUP   = "exec-group"
	SCOPE_CLIENT_GROUP = "client-group"
	SCOPE_CLIENT       = "client"
	SCOPE_ITER         = "iter"
	SCOPE_TRX          = "trx"
	SCOPE_STATEMENT    = "statement"
	SCOPE_ROW          = "row" // special: INSERT INTO t VALUES (@d), (@d), ...
	SCOPE_VALUE        = "value"
)

func (rl RunLevel) array() []uint {
	return []uint{
		uint(rl.Query), // 0
		rl.Trx,         // 1
		0,              // 2 ITER (data scope)
		rl.Client,      // 3
		rl.ClientGroup, // 4
		rl.ExecGroup,   // 5
		1,              // 6 WORKLOAD not counted
		rl.Stage,       // 7 STAGE
		1,              // 8 GLOBAL
	}
}

func (rl RunLevel) GreaterThan(prev RunLevel, s string) bool {
	switch s {
	case SCOPE_VALUE:
		return true // all value scoped @d are unique
	case SCOPE_ROW:
		s = SCOPE_STATEMENT // row scoped @d per statement
	case SCOPE_ITER:
		s = SCOPE_CLIENT // iter scoped @d per client
	}
	n := RunLevelNumber(s)
	now := rl.array()
	before := prev.array()
	for i := n; i < uint(len(now)); i++ {
		if now[i] > before[i] {
			return true
		}
	}
	return false
}

var runlevelNumber = map[string]uint{
	SCOPE_STATEMENT:    0, // data.STATEMENT | These four must
	SCOPE_TRX:          1, // data.TRX       | match, else
	SCOPE_ITER:         2, // data.ITER      | NewScopedGenerator
	SCOPE_CLIENT:       3, // data.CONN      | needs to be updated
	SCOPE_CLIENT_GROUP: 4,
	SCOPE_EXEC_GROUP:   5,
	SCOPE_WORKLOAD:     6,
	SCOPE_STAGE:        7,
	SCOPE_GLOBAL:       8,
}

func RunLevelNumber(s string) uint {
	n, ok := runlevelNumber[s]
	if !ok {
		panic("invalid run level: " + s)
	}
	return n
}

func RunLevelIsValid(s string) bool {
	_, ok := runlevelNumber[s]
	return ok
}

var (
	CPUProfile io.Writer // --cpu-profile FILE
	Debugging  = false
	debugLog   = log.New(os.Stderr, "", log.LstdFlags|log.Lmicroseconds)
)

func Debug(msg string, v ...interface{}) {
	if !Debugging {
		return
	}
	_, file, line, _ := runtime.Caller(1)
	msg = fmt.Sprintf("DEBUG %s:%d %s", path.Base(file), line, msg)
	debugLog.Printf(msg, v...)
}

var MakeHTTPClient func() *http.Client = func() *http.Client {
	tr := &http.Transport{
		MaxIdleConns:    1,
		IdleConnTimeout: 1 * time.Hour,
	}
	return &http.Client{Transport: tr}
}

func Bool(s string) bool {
	v := strings.ToLower(s)
	return v == "true" || v == "yes" || v == "on" || v == "aye"
}

func BoolString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

var portRe = regexp.MustCompile(`:\d+$`)

// WithPort return s:p if s doesn't have port suffix p.
// p must not have a colon prefix.
func WithPort(s, p string) string {
	if portRe.MatchString(s) {
		return s
	}
	return s + ":" + p
}

// Uint returns s as a unit presuming s has already been validated.
func Uint(s string) uint {
	i, _ := strconv.ParseUint(s, 10, 64)
	return uint(i)
}

var SystemParams = map[string]string{}

func init() {
	SystemParams["CPU_CORES"] = strconv.Itoa(runtime.NumCPU())
}

const (
	Eabort     = 0         // default (set to clear default flags)
	Ereconnect = 1 << iota // reconnect, next iter
	Econtinue              // don't reconnect, continue next iter
	Esilent                // don't repot error or reconnect
	Erollback              // execute ROLLBACK if in trx
)

var MySQLErrorHandling = map[uint16]byte{
	1046: Eabort,                // no database selected
	1062: Eabort,                // duplicate key
	1064: Eabort,                // You have an error in your SQL syntax
	1146: Eabort,                // table doesn't exist
	1205: Erollback | Econtinue, // lock wait timeout; no automatic rollback (innodb_rollback_on_timeout=OFF by default)
	1213: Econtinue,             // deadlock; automatic rollback
	1290: Erollback | Econtinue, // read-only (server is running with the --read-only option so it cannot execute this statement)
	1317: Econtinue,             // query killed (Query execution was interrupted)
	1836: Erollback | Econtinue, // read-only (Running in read-only mode)
}

var ModifyDB func(*sql.DB, RunLevel)
