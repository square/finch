// Copyright 2022 Block, Inc.

package compute

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/square/finch"
	"github.com/square/finch/config"
	"github.com/square/finch/stats"
)

// Coordinator runs each stage using local and remote instances.
//
//	Local (Coordinator) (config.bind) | Next stage
//	└──Instance					   | Run stage
//	   └──Stage (Warmup)			   | Run clients
//	      └──Client_1				   | Execute queries
//	      ...						   | ...
//	      └──Client_N				   | Execute queries
//	   └──Stage (Benchmark)
//	      └──Client_1
//	      ...
//	      └──Client_N
//	└──(httpServer)<-Remote_1
//	└──(httpServer)<-Remote_N
//
//	Remote_1 (Coordinator) -> config.server
//	└──Instance
//	   └──Stage
//	      └──Client_1
type Coordinator interface {
	Boot(context.Context, config.File) error
	Run(context.Context) error
	Stop()
}

type remote struct {
	name string
	// @todo struct might not be needed
}

type ack struct {
	name string // "" for local, else remote.name
	err  error
}

// Local coordinates multiple instances.
type Local struct {
	haveRemotes bool
	httpServer  *http.Server
	nInstances  uint
	bootChan    chan ack
	doneChan    chan ack

	*sync.Mutex
	local   *Instance
	first   string
	remotes map[string]*remote // keyed on instance name

	cfg   config.File
	stage string
	done  bool
	stop  bool

	ag *stats.Ag
}

var _ Coordinator = &Local{}

func NewCoordinator(cfg config.File) *Local {
	c := &Local{
		cfg:        cfg,
		Mutex:      &sync.Mutex{},
		nInstances: cfg.Compute.Instances,
		bootChan:   make(chan ack, cfg.Compute.Instances),
		doneChan:   make(chan ack, cfg.Compute.Instances),
	}

	return c
}

func (c *Local) Stop() {
	if c.local != nil {
		c.local.Stop()
	}

	if !c.haveRemotes {
		return
	}

	c.Lock()
	c.stop = true
	c.Unlock()

	for {
		c.Lock()
		n := len(c.remotes)
		c.Unlock()
		if n == 0 {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	c.httpServer.Close()
}

func (c *Local) Boot(ctx context.Context, cfg config.File) error {
	// Boot stats aggregator first to ensure we can report stats, which is kind of
	// the whole point of benchmarking. Plus, the "ag" receives stats from all clients
	// (local and remote).
	ag, err := stats.NewAg(c.cfg.Compute.Instances, c.cfg.Stats)
	if err != nil {
		return err
	}
	c.ag = ag

	// Boot local instances first because if this doesn't work, then remotes should
	// fail to boot, too, since they're sent the same config
	if !config.True(cfg.Compute.DisableLocal) {
		c.local = NewInstance(
			cfg.Compute.Name,
			stats.NewCollector(cfg.Stats, "local", c.ag.Chan()),
		)
		if err := c.local.Boot(ctx, cfg); err != nil {
			return err
		}
	}

	// How many compute instances do we have? And are any remotes?
	nRemotes := cfg.Compute.Instances
	if nRemotes > 0 && c.local != nil {
		nRemotes -= 1
	}
	c.haveRemotes = nRemotes > 0
	log.Printf("Local coordinator: %d instances (%d remote)", c.nInstances, nRemotes)

	// If no remotes, then we're done booting; coordinator is kind of useless overhead
	// but other funcs will check c.haveRemotes and avoid the extra complexity of
	// coordinating remote compute instances.
	if !c.haveRemotes {
		return nil
	}

	// ----------------------------------------------------------------------
	// Remote compute instances

	c.remotes = map[string]*remote{}

	// HTTP server that remote instances calls
	mux := http.NewServeMux()
	mux.HandleFunc("/boot", c.remoteBoot)
	mux.HandleFunc("/file", c.remoteFile)
	mux.HandleFunc("/run", c.remoteRun)
	mux.HandleFunc("/stats", c.remoteStats)
	mux.HandleFunc("/stop", c.remoteStop)
	c.httpServer = &http.Server{
		Addr:    cfg.Compute.Bind,
		Handler: mux,
	}
	go c.httpServer.ListenAndServe() // @todo

	// Wait for the required number remotes to boot
	for nRemotes > 0 {
		log.Printf("Have %d remotes, need %d...", len(c.remotes), nRemotes)
		select {
		case ack := <-c.bootChan:
			if ack.err != nil {
				return fmt.Errorf("Remote %s error on boot: %s", ack.name, ack.err)
			}
			log.Printf("Remote %s booted", ack.name)
			nRemotes -= 1
		case <-ctx.Done():
			return nil
		}
	}
	log.Println("All instances have booted")

	return nil
}

// Run runs all the stages on all the instances (local and remote).
func (c *Local) Run(ctx context.Context) error {
	// Run setup on local only
	if !c.cfg.Setup.Disable && len(c.cfg.Setup.Workload) > 0 && c.local != nil {
		if err := c.local.Run(ctx, finch.STAGE_SETUP); err != nil {
			return err
		}
	}

	// ----------------------------------------------------------------------
	// BENCHMARK
	// ----------------------------------------------------------------------
	// Run warmup and benchmark on local and remotes (if any)
	if !c.cfg.Warmup.Disable && len(c.cfg.Warmup.Workload) > 0 {
		if err := c.run(ctx, finch.STAGE_WARMUP); err != nil {
			return err
		}
	}
	if !c.cfg.Benchmark.Disable && len(c.cfg.Benchmark.Workload) > 0 {
		go c.ag.Run()
		if err := c.run(ctx, finch.STAGE_BENCHMARK); err != nil {
			return err
		}

		// Stop stats aggregator and wait for it to print last stats
		c.ag.Done()
	}
	// ----------------------------------------------------------------------

	// Tell remotes they're done
	c.Lock()
	c.done = true
	c.Unlock()

	// Run cleanup on local only
	if !c.cfg.Cleanup.Disable && len(c.cfg.Cleanup.Workload) > 0 && c.local != nil {
		if err := c.local.Run(ctx, finch.STAGE_CLEANUP); err != nil {
			return err
		}
	}

	// Wait for all remotes to disconnected
	if !c.haveRemotes {
		return nil
	}

WAIT:
	for {
		select {
		case <-ctx.Done():
			break WAIT
		default:
		}
		c.Lock()
		connected := len(c.remotes)
		c.Unlock()
		if connected == 0 {
			break WAIT
		}
		log.Printf("%d remotes still connected...", connected)
		time.Sleep(1 * time.Second)
	}

	return nil
}

func (c *Local) run(ctx context.Context, stageName string) error {
	finch.Debug("run %s", stageName)
	c.Lock()
	c.stage = stageName
	c.Unlock()
	defer func() {
		c.Lock()
		c.stage = ""
		c.Unlock()
	}()

	finch.Debug("run %s", stageName)
	if c.local != nil {
		go func() {
			a := ack{name: "Local"}
			a.err = c.local.Run(ctx, stageName)
			c.doneChan <- a
		}()
	}

	// Wait for instancse to finish running stage
	running := c.nInstances
	for running > 0 {
		log.Printf("%d instances running...", running)
		select {
		case ack := <-c.doneChan:
			running -= 1
			if ack.err != nil {
				log.Printf("%s error running stage %s: %s", ack.name, stageName, ack.err)
			} else {
				log.Printf("%s completed stage %s", ack.name, stageName)
			}
		case <-ctx.Done():
			return nil // @stop instances?
		}
	}

	return nil
}

// --------------------------------------------------------------------------

// ServeHTTP allows the API to statisfy the http.HandlerFunc interface.
func (c *Local) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c.httpServer.Handler.ServeHTTP(w, r)
}

func (c *Local) remoteBoot(w http.ResponseWriter, r *http.Request) {
	name, ok := clientName(w, r)
	if !ok {
		return // clientName() wrote error response
	}

	get, ok := method(w, r)
	if !ok {
		return // method() wrote error respone
	}

	// Load remote state
	c.Lock()
	defer c.Unlock()
	if _, ok := c.remotes[name]; !ok {
		if get {
			// New remote instance
			//   @todo limit number of remote instances
			//   @todo handle duplicate names
			c.remotes[name] = &remote{name: name}
		} else {
			unknownRemote(w, name)
			return
		}
	}

	// POST /boot: remote is ack'ing that previous GET /boot completed successfully
	if !get {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			log.Printf("error reading error from remote: %s", err)
			return
		}
		r.Body.Close()
		w.WriteHeader(http.StatusOK)

		var remoteErr error
		if string(body) != "" {
			remoteErr = fmt.Errorf("%s", string(body))
		}
		c.bootChan <- ack{name: name, err: remoteErr}
		return
	}

	// GET /boot: remote is booting, waiting to receive config.File
	log.Printf("Remote %s booting\n", name)
	json.NewEncoder(w).Encode(c.cfg)
}

func (c *Local) remoteStop(w http.ResponseWriter, r *http.Request) {
	name, ok := clientName(w, r)
	if !ok {
		return // clientName() wrote error response
	}

	get, ok := method(w, r)
	if !ok {
		return // method() wrote error respone
	}

	c.Lock()
	defer c.Unlock()
	if _, ok := c.remotes[name]; !ok {
		// @todo: unknown instance, didn't POST /boot first
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	// GET /stop: remote is checking if it's ok to keep running
	if get {
		c.Lock()
		stop := c.stop
		c.Unlock()
		if stop {
			w.WriteHeader(http.StatusNoContent) // remote should stop
		} else {
			w.WriteHeader(http.StatusOK) // remote is ok to keep running
		}
		return
	}

	// POST /stop: remote is disconnecting
	delete(c.remotes, name)
	w.WriteHeader(http.StatusOK)
	log.Printf("Remote %s disconnected", name)
}

func (c *Local) remoteFile(w http.ResponseWriter, r *http.Request) {
	name, ok := clientName(w, r)
	if !ok {
		return // clientName() wrote error response
	}
	c.Lock()
	defer c.Unlock()
	if _, ok := c.remotes[name]; !ok {
		// @todo: unknown instance, didn't POST /boot first
		w.WriteHeader(http.StatusUnauthorized) // unknown remote
		return
	}

	// Parse file ref 'stage=...&i=...' from URL
	q := r.URL.Query()
	finch.Debug("remoteFile params %+v", q)
	vals, ok := q["stage"]
	if !ok {
		http.Error(w, "missing stage param in URL query: ?stage=...", http.StatusBadRequest)
		return
	}
	if len(vals) == 0 {
		http.Error(w, "stage param has no value, expected stage name", http.StatusBadRequest)
		return
	}
	stage := clean(vals[0])

	vals, ok = q["i"]
	if !ok {
		http.Error(w, "missing i param in URL query: i=N", http.StatusBadRequest)
		return
	}
	if len(vals) == 0 {
		http.Error(w, "i param has no value, expected file number", http.StatusBadRequest)
		return
	}
	i, err := strconv.Atoi(clean(vals[0]))
	if err != nil {
		http.Error(w, "i param is not an integer", http.StatusBadRequest)
		return
	}
	if i < 0 {
		http.Error(w, "i param is negative", http.StatusBadRequest)
		return
	}

	// Validate the stage and file index
	var trx []config.Trx
	switch stage {
	case finch.STAGE_SETUP:
		trx = c.cfg.Setup.Workload
	case finch.STAGE_WARMUP:
		trx = c.cfg.Warmup.Workload
	case finch.STAGE_BENCHMARK:
		trx = c.cfg.Benchmark.Workload
	case finch.STAGE_CLEANUP:
		trx = c.cfg.Cleanup.Workload
	default:
		http.Error(w, "invalid stage: "+stage, http.StatusBadRequest)
		return
	}
	if len(trx) == 0 {
		http.Error(w, "stage "+stage+" has no files", http.StatusBadRequest)
		return
	}
	if i > len(trx)-1 {
		http.Error(w, "i param out of range for stage "+stage, http.StatusBadRequest)
		return
	}

	log.Printf("Sending stage %s file %s to %s...", stage, trx[i].File, name)

	// Read file and send it to the remote instance
	bytes, err := ioutil.ReadFile(trx[i].File)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
	w.Write(bytes)

	log.Printf("Sent stage %s file %s to %s", stage, trx[i].File, name)
}

func (c *Local) remoteRun(w http.ResponseWriter, r *http.Request) {
	name, ok := clientName(w, r)
	if !ok {
		return // clientName() wrote error response
	}

	get, ok := method(w, r)
	if !ok {
		return // method() wrote error respone
	}

	c.Lock()
	if _, ok := c.remotes[name]; !ok {
		log.Printf("Uknown remote: %s", name)
		w.WriteHeader(http.StatusUnauthorized) // unknown remote
		c.Unlock()
		return
	}
	c.Unlock()

	// If POST, remote is ack'ing that previous GET /run started or completed successfully
	if !get {
		finch.Debug("remote %s: POST /run", name)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			log.Printf("error reading error from remote: %s", err)
			c.Unlock()
			return
		}
		r.Body.Close()
		w.WriteHeader(http.StatusOK)
		var remoteErr error
		if string(body) != "" {
			remoteErr = fmt.Errorf("%s", string(body))
		}
		c.doneChan <- ack{name: name, err: remoteErr}
		return
	}

	// Remote is waiting for next stage to run
	log.Printf("Remote %s waiting to start...", name)
	for {
		c.Lock()
		done := c.done
		stop := c.stop
		stageName := c.stage
		c.Unlock()
		if done || stop {
			log.Printf("Disconnecting remote %s", name)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if stageName != "" {
			break
		}
		time.Sleep(300 * time.Millisecond)
	}

	log.Printf("Starting remote %s on stage %s\n", name, c.stage)
	w.Write([]byte(c.stage))
}

func (c *Local) remoteStats(w http.ResponseWriter, r *http.Request) {
	name, ok := clientName(w, r)
	if !ok {
		return // clientName() wrote error response
	}
	c.Lock()
	defer c.Unlock()
	if _, ok := c.remotes[name]; !ok {
		// @todo: unknown instance, didn't POST /boot first
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("error reading error from remote: %s", err)
		return
	}
	r.Body.Close()
	w.WriteHeader(http.StatusOK)

	var s stats.Stats
	if err := json.Unmarshal(body, &s); err != nil {
		log.Printf("Invalid stats from %s: %s", name, err)
	}
	c.ag.Chan() <- s // ag debug-prints the stats on recv
}

// --------------------------------------------------------------------------

func clientName(w http.ResponseWriter, r *http.Request) (string, bool) {
	q := r.URL.Query()
	if len(q) == 0 {
		http.Error(w, "missing URL query: ?name=...", http.StatusBadRequest)
		return "", false
	}
	vals, ok := q["name"]
	if !ok {
		http.Error(w, "missing name param in URL query: ?name=...", http.StatusBadRequest)
		return "", false
	}
	if len(vals) == 0 {
		http.Error(w, "name param has no value, expected instance name", http.StatusBadRequest)
		return "", false
	}
	return clean(vals[0]), true
}

func unknownRemote(w http.ResponseWriter, name string) {
	w.WriteHeader(http.StatusUnauthorized) // unknown remote
}

func method(w http.ResponseWriter, r *http.Request) (get bool, ok bool) {
	// GET or POST
	get = true
	ok = true
	switch r.Method {
	case http.MethodGet: // allowed
	case http.MethodPost: // allowed
		get = false
	default:
		ok = false
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	return
}

// clean removes \n\r to avoid code scanning alert "Log entries created from user input".
func clean(s string) string {
	c := strings.Replace(s, "\n", "", -1)
	return strings.Replace(c, "\r", "", -1)
}
