// Copyright 2022 Block, Inc.

package finch

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path"
	"runtime"
	"time"
)

const VERSION = "1.0.0"

const (
	STAGE_SETUP     = "setup"
	STAGE_WARMUP    = "warmup"
	STAGE_BENCHMARK = "benchmark"
	STAGE_CLEANUP   = "cleanup"
)

// global
// └──stage
//    └──sequence
//       └──client group
//          └──client
//             └──trx
//                └──statment

const (
	SCOPE_STATEMENT   = "s"
	SCOPE_TRANSACTION = "t"
	SCOPE_WORKLOAD    = "w"
	SCOPE_GLOBAL      = "g"
)

func Stages() []string {
	return []string{STAGE_SETUP, STAGE_WARMUP, STAGE_BENCHMARK, STAGE_CLEANUP}
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
