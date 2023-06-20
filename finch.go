// Copyright 2023 Block, Inc.

package finch

import (
	"fmt"
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
	Iter          uint
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
	SCOPE_TRX          = "trx"
	SCOPE_STATEMENT    = "statement"
)

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
	return v == "true" || v == "yes" || v == "on" || v == "aye"
}

func BoolString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

var portRe = regexp.MustCompile(`:\d+$`)

func WithPort(s, p string) string {
	if portRe.MatchString(s) {
		return s
	}
	return s + ":" + p
}

func Uint(s string) uint {
	if s == "" {
		return 0
	}
	i, _ := strconv.ParseUint(s, 10, 64)
	return uint(i)
}

var SystemParams = map[string]string{}

func init() {
	SystemParams["CPU_CORES"] = strconv.Itoa(runtime.NumCPU())
}
