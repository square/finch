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

type Compute interface {
	Boot(context.Context, config.File) error
	Run(context.Context) error
	Stop()
}

// Server coordinates instances: the local and any remotes. Server implements
// Compute so server.Server (the Finch core server) can run as a client or server.
type Server struct {
	haveRemotes bool
	httpServer  *http.Server
	nInstances  uint
	bootChan    chan ack
	doneChan    chan ack

	*sync.Mutex
	local   *Local
	first   string
	remotes map[string]*remote // keyed on instance name

	cfg   config.File
	stage string
	done  bool
	stop  bool

	stats *stats.Collector
}

var _ Compute = &Server{}

type remote struct {
	name string
	// @todo struct might not be needed
}

type ack struct {
	name string // "" for local, else remote.name
	err  error
}

func NewServer(cfg config.File) *Server {
	return &Server{
		cfg:        cfg,
		Mutex:      &sync.Mutex{},
		nInstances: cfg.Compute.Instances,
		bootChan:   make(chan ack, cfg.Compute.Instances),
		doneChan:   make(chan ack, cfg.Compute.Instances),
	}
}

func (s *Server) Stop() {
	if s.local != nil {
		s.local.Stop()
	}

	if !s.haveRemotes {
		return
	}

	s.Lock()
	s.stop = true
	s.Unlock()

	for {
		s.Lock()
		n := len(s.remotes)
		s.Unlock()
		if n == 0 {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	s.httpServer.Close()
}

func (s *Server) Boot(ctx context.Context, cfg config.File) error {
	var err error

	// Boot stats aggregator first to ensure we can report stats, which is kind of
	// the whole point of benchmarking. Plus, the "ag" receives stats from all clients
	// (local and remote).
	s.stats, err = stats.NewCollector(s.cfg.Stats, "local", s.cfg.Compute.Instances)
	if err != nil {
		return err
	}

	// Boot local instances first because if this doesn't work, then remotes should
	// fail to boot, too, since they're sent the same config
	if !config.True(cfg.Compute.DisableLocal) {
		s.local = NewLocal(cfg.Compute.Name, s.stats)
		if err := s.local.Boot(ctx, cfg); err != nil {
			return err
		}
	}

	// How many compute instances do we have? And are any remotes?
	nRemotes := cfg.Compute.Instances
	if nRemotes > 0 && s.local != nil {
		nRemotes -= 1
	}
	s.haveRemotes = nRemotes > 0
	log.Printf("Compute coordinator: %d instances (%d remote)", s.nInstances, nRemotes)

	// If no remotes, then we're done booting; coordinator is kind of useless overhead
	// but other funcs will check s.haveRemotes and avoid the extra complexity of
	// coordinating remote compute instances.
	if !s.haveRemotes {
		return nil
	}

	// ----------------------------------------------------------------------
	// Remote compute instances

	s.remotes = map[string]*remote{}

	// HTTP server that remote instances calls
	mux := http.NewServeMux()
	mux.HandleFunc("/boot", s.remoteBoot)
	mux.HandleFunc("/file", s.remoteFile)
	mux.HandleFunc("/run", s.remoteRun)
	mux.HandleFunc("/stats", s.remoteStats)
	mux.HandleFunc("/stop", s.remoteStop)
	s.httpServer = &http.Server{
		Addr:    cfg.Compute.Bind,
		Handler: mux,
	}
	go s.httpServer.ListenAndServe() // @todo

	// Wait for the required number remotes to boot
	for nRemotes > 0 {
		log.Printf("Have %d remotes, need %d...", len(s.remotes), nRemotes)
		select {
		case ack := <-s.bootChan:
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
func (s *Server) Run(ctx context.Context) error {
	// Run setup on local only
	if !s.cfg.Setup.Disable && len(s.cfg.Setup.Trx) > 0 && s.local != nil {
		if err := s.local.Run(ctx, finch.STAGE_SETUP); err != nil {
			return err
		}
	}

	// ----------------------------------------------------------------------
	// BENCHMARK
	// ----------------------------------------------------------------------
	// Run warmup and benchmark on local and remotes (if any)
	if !s.cfg.Warmup.Disable && len(s.cfg.Warmup.Trx) > 0 {
		if err := s.run(ctx, finch.STAGE_WARMUP); err != nil {
			return err
		}
	}
	if !s.cfg.Benchmark.Disable && len(s.cfg.Benchmark.Trx) > 0 {
		if err := s.run(ctx, finch.STAGE_BENCHMARK); err != nil {
			return err
		}
	}
	// ----------------------------------------------------------------------

	// Tell remotes they're done
	s.Lock()
	s.done = true
	s.Unlock()

	// Run cleanup on local only
	if !s.cfg.Cleanup.Disable && len(s.cfg.Cleanup.Trx) > 0 && s.local != nil {
		if err := s.local.Run(ctx, finch.STAGE_CLEANUP); err != nil {
			return err
		}
	}

	// Wait for all remotes to disconnected
	if !s.haveRemotes {
		return nil
	}

WAIT:
	for {
		select {
		case <-ctx.Done():
			break WAIT
		default:
		}
		s.Lock()
		connected := len(s.remotes)
		s.Unlock()
		if connected == 0 {
			break WAIT
		}
		log.Printf("%d remotes still connected...", connected)
		time.Sleep(1 * time.Second)
	}

	return nil
}

func (s *Server) run(ctx context.Context, stageName string) error {
	finch.Debug("run %s", stageName)
	s.Lock()
	s.stage = stageName
	s.Unlock()
	defer func() {
		s.Lock()
		s.stage = ""
		s.Unlock()
	}()

	finch.Debug("run %s", stageName)
	if s.local != nil {
		go func() {
			a := ack{name: "Local"}
			a.err = s.local.Run(ctx, stageName)
			s.doneChan <- a
		}()
	}

	// Wait for instancse to finish running stage
	running := s.nInstances
	for running > 0 {
		log.Printf("%d instances running...", running)
		select {
		case ack := <-s.doneChan:
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
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.httpServer.Handler.ServeHTTP(w, r)
}

func (s *Server) remoteBoot(w http.ResponseWriter, r *http.Request) {
	name, ok := clientName(w, r)
	if !ok {
		return // clientName() wrote error response
	}

	get, ok := method(w, r)
	if !ok {
		return // method() wrote error respone
	}

	// Load remote state
	s.Lock()
	defer s.Unlock()
	if _, ok := s.remotes[name]; !ok {
		if get {
			// New remote instance
			//   @todo limit number of remote instances
			//   @todo handle duplicate names
			s.remotes[name] = &remote{name: name}
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
		s.bootChan <- ack{name: name, err: remoteErr}
		return
	}

	// GET /boot: remote is booting, waiting to receive config.File
	log.Printf("Remote %s booting\n", name)
	json.NewEncoder(w).Encode(s.cfg)
}

func (s *Server) remoteStop(w http.ResponseWriter, r *http.Request) {
	name, ok := clientName(w, r)
	if !ok {
		return // clientName() wrote error response
	}

	get, ok := method(w, r)
	if !ok {
		return // method() wrote error respone
	}

	s.Lock()
	defer s.Unlock()
	if _, ok := s.remotes[name]; !ok {
		// @todo: unknown instance, didn't POST /boot first
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	// GET /stop: remote is checking if it's ok to keep running
	if get {
		s.Lock()
		stop := s.stop
		s.Unlock()
		if stop {
			w.WriteHeader(http.StatusNoContent) // remote should stop
		} else {
			w.WriteHeader(http.StatusOK) // remote is ok to keep running
		}
		return
	}

	// POST /stop: remote is disconnecting
	delete(s.remotes, name)
	w.WriteHeader(http.StatusOK)
	log.Printf("Remote %s disconnected", name)
}

func (s *Server) remoteFile(w http.ResponseWriter, r *http.Request) {
	name, ok := clientName(w, r)
	if !ok {
		return // clientName() wrote error response
	}
	s.Lock()
	defer s.Unlock()
	if _, ok := s.remotes[name]; !ok {
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
		trx = s.cfg.Setup.Trx
	case finch.STAGE_WARMUP:
		trx = s.cfg.Warmup.Trx
	case finch.STAGE_BENCHMARK:
		trx = s.cfg.Benchmark.Trx
	case finch.STAGE_CLEANUP:
		trx = s.cfg.Cleanup.Trx
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

func (s *Server) remoteRun(w http.ResponseWriter, r *http.Request) {
	name, ok := clientName(w, r)
	if !ok {
		return // clientName() wrote error response
	}

	get, ok := method(w, r)
	if !ok {
		return // method() wrote error respone
	}

	s.Lock()
	if _, ok := s.remotes[name]; !ok {
		log.Printf("Uknown remote: %s", name)
		w.WriteHeader(http.StatusUnauthorized) // unknown remote
		s.Unlock()
		return
	}
	s.Unlock()

	// If POST, remote is ack'ing that previous GET /run started or completed successfully
	if !get {
		finch.Debug("remote %s: POST /run", name)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			log.Printf("error reading error from remote: %s", err)
			s.Unlock()
			return
		}
		r.Body.Close()
		w.WriteHeader(http.StatusOK)
		var remoteErr error
		if string(body) != "" {
			remoteErr = fmt.Errorf("%s", string(body))
		}
		s.doneChan <- ack{name: name, err: remoteErr}
		return
	}

	// Remote is waiting for next stage to run
	log.Printf("Remote %s waiting to start...", name)
	for {
		s.Lock()
		done := s.done
		stop := s.stop
		stageName := s.stage
		s.Unlock()
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

	log.Printf("Starting remote %s on stage %s\n", name, s.stage)
	w.Write([]byte(s.stage))
}

func (s *Server) remoteStats(w http.ResponseWriter, r *http.Request) {
	name, ok := clientName(w, r)
	if !ok {
		return // clientName() wrote error response
	}
	s.Lock()
	defer s.Unlock()
	if _, ok := s.remotes[name]; !ok {
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

	var in stats.Instance
	if err := json.Unmarshal(body, &in); err != nil {
		log.Printf("Invalid stats from %s: %s", name, err)
		return
	}
	s.stats.Recv(in)
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
