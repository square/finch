// Copyright 2022 Block, Inc.

package finch

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path"
	"runtime"
	"strings"
	"time"
)

const VERSION = "1.0.0"

const (
	STAGE_SETUP     = "setup"
	STAGE_WARMUP    = "warmup"
	STAGE_BENCHMARK = "benchmark"
	STAGE_CLEANUP   = "cleanup"
)

// Execution is sequenced by execution group: sequential or concurrent.
// All clients in an execution group execute together, concurrently.
// Client groups vary the number of clients and trx those clients execute (within an exec group).

// exec group 1
// └──client group 1  [trx1, trx2]
//    └── client 1
//    └── client 2
//    └── cilent 3
//    └── client 4
// exec group 2
// └──client group 2  [trx2]
// |  └── client 1
// |  └── client 2
// └──client group 2  [trx3]
//    └── client 1
//    └── client 2

// sequence is an ordered set of trx
//   by workload (setup)
//   by clients (client gorups)
//   all clients, all workloads

// global
// └──stage*              (might repeat: iter 2, so workload scope needed)
//    └──workload
//       └──exec group
//          └──client group
//             └──client*
//                └──trx         } trx set
//                   └──statment }

type RunLevel struct {
	Stage         string
	ExecGroup     uint
	ExecGroupName string
	ClientGroup   uint
	Client        uint
	Iter          uint
	Trx           uint
	TrxName       string
	Query         uint64
}

func (s RunLevel) String() string {
	return fmt.Sprintf("%s/e%d(%s)/g%d/c%d/t%d(%s)/q%d",
		s.Stage, s.ExecGroup, s.ExecGroupName, s.ClientGroup, s.Client, s.Trx, s.TrxName, s.Query)
}

func (s RunLevel) ClientId() string {
	return fmt.Sprintf("%s/e%d(%s)/g%d/c%d",
		s.Stage, s.ExecGroup, s.ExecGroupName, s.ClientGroup, s.Client)
}

type ExecCount [6]uint

const (
	STAGE byte = iota
	EXEC_GROUP
	CLIENT_GROUP
	ITER
	TRX
	QUERY
)

const NOOP_COLUMN = "_"

const (
	EXEC_SEQUENTIAL = "sequential"
	EXEC_CONCURRENT = "concurrent"
)
const (
	TRX_STMT  = byte(0x0)
	TRX_BEGIN = byte(0x1)
	TRX_END   = byte(0x2)
)

const (
	SCOPE_GLOBAL       = "global"
	SCOPE_STAGE        = "stage"
	SCOPE_WORKLOAD     = "workload"
	SCOPE_EXEC_GROUP   = "exec-group"
	SCOPE_CLIENT_GROUP = "client-group"
	SCOPE_CLIENT       = "client"
	SCOPE_TRX          = "trx"
	SCOPE_STATEMENT    = "statement"
)

var StageNumber = map[string]uint{
	"setup":     1,
	"warmup":    2,
	"benchmark": 3,
	"cleanup":   4,
}

var (
	Debugging = false
	debugLog  = log.New(os.Stderr, "", log.LstdFlags|log.Lmicroseconds)
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
	return v == "true" || v == "yes" || v == "enable" || v == "enabled"
}

func BoolString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
