package compute

import (
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

type API struct {
	httpServer *http.Server
	*sync.Mutex
	stage *metaStage
}

const (
	ready byte = iota
	booting
	runnable
	running
)

var retryWait = 100 * time.Millisecond

type metaStage struct {
	cfg        config.Stage
	sid        string           // stage ID
	nInstances uint             // shortcut for config.compute.instances
	bootChan   chan ack         // 1. <-remote after booting stage
	runChan    chan struct{}    // 2. server closes to signal remotes to run
	doneChan   chan ack         // 3. <-remote after running stage
	stats      *stats.Collector // receives stats from remotes while running
	local      *Local           // local instance if not config.compute.disable-local
	reset      bool
	remotes    map[string]*remote
}

// Remote is a remote compute (rc): Finch running as a client on another computer
// connecting to this computer (which is the server).
type remote struct {
	name  string
	stage *metaStage
	state byte
}

func NewAPI(addr string) *API {
	a := &API{
		Mutex: &sync.Mutex{},
	}

	// HTTP server that remote instances calls
	mux := http.NewServeMux()
	mux.HandleFunc("/boot", a.boot)
	mux.HandleFunc("/file", a.file)
	mux.HandleFunc("/run", a.run)
	mux.HandleFunc("/stats", a.stats)
	mux.HandleFunc("/stop", a.stop)
	a.httpServer = &http.Server{
		Addr:    addr,
		Handler: mux,
	}
	go a.httpServer.ListenAndServe() // @todo
	return a
}

// ServeHTTP implements the http.HandlerFunc interface.
func (a *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.httpServer.Handler.ServeHTTP(w, r)
}

func (a *API) Stage(s *metaStage) error {
	if a.stage != nil {
		a.Lock()
		a.stage.reset = true   // signal resetClient
		close(a.stage.runChan) // unblock clients in GET /run
		a.Unlock()

		timeout := time.After(2 * time.Second)
		for {
			time.Sleep(100 * time.Millisecond)
			select {
			case <-timeout:
				return fmt.Errorf("timeout waiting for remotes to reset")
			default:
			}
			a.Lock()
			n := len(a.stage.remotes)
			a.Unlock()
			if n == 0 {
				break
			}
		}
	}
	a.Lock()
	a.stage = s
	a.Unlock()
	return nil
}

func (a *API) resetClient(rc *remote, w http.ResponseWriter, force bool) bool {
	a.Lock()
	defer a.Unlock()
	if a.stage.reset == false && force == false {
		return false
	}
	delete(rc.stage.remotes, rc.name)
	w.WriteHeader(http.StatusResetContent)
	return true
}

func (a *API) boot(w http.ResponseWriter, r *http.Request) {
	rc, get, ok := a.remote(w, r, true) // true == allow new remotes on GET /boot
	if !ok {
		return // remote() wrote error response
	}

	if get {
		// GET /boot: remote is booting, waiting to receive config.File
		if rc.state != ready {
			w.WriteHeader(http.StatusPreconditionFailed)
			return
		}

		// Wait until there's a stage and a slot left to run it (and stage isn't being reset)
		log.Printf("Remote %s ready to boot\n", rc.name)
		for {
			a.Lock()
			stage := a.stage // copy ptr but hold lock so len(stage.remotes) is correct
			if stage != nil && len(stage.remotes) < int(stage.nInstances) && !stage.reset {
				stage.remotes[rc.name] = rc // take the slot
				a.Unlock()
				rc.stage = stage                     // save ptr to stage
				rc.state = booting                   // advance remote state
				json.NewEncoder(w).Encode(stage.cfg) // send stage config
				return
			}
			a.Unlock()
			time.Sleep(retryWait)
		}
	} else {
		// POST /boot: remote is ack'ing previous GET /boot; body is error message, if any
		if rc.state != booting {
			w.WriteHeader(http.StatusPreconditionFailed)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			log.Printf("error reading error from remote: %s", err)
			return
		}
		r.Body.Close()
		w.WriteHeader(http.StatusOK)

		// Remote might fail to boot. If that's the case, do not advance its state;
		// it should call GET /boot again to reset itself and try again.
		var remoteErr error
		if string(body) != "" {
			// Don't advance state: remote failed to boot, so it's not ready to run
			remoteErr = fmt.Errorf("%s", string(body))
		} else {
			rc.state = runnable // advance remote state (successful boot)
		}
		rc.stage.bootChan <- ack{name: rc.name, err: remoteErr}
	}
}

func (a *API) file(w http.ResponseWriter, r *http.Request) {
	rc, _, ok := a.remote(w, r, false)
	if !ok {
		return // remote() wrote error response
	}

	if rc.state != booting {
		w.WriteHeader(http.StatusPreconditionFailed)
		return
	}

	// Parse file ref 'stage=...&i=...' from URL
	q := r.URL.Query()
	finch.Debug("file params %+v", q)
	vals, ok := q["stage"]
	if !ok {
		http.Error(w, "missing stage param in URL query: ?stage=...", http.StatusBadRequest)
		return
	}
	if len(vals) == 0 {
		http.Error(w, "stage param has no value, expected stage name", http.StatusBadRequest)
		return
	}

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
	s := rc.stage.cfg // shortcut
	if i > len(s.Trx)-1 {
		http.Error(w, "i param out of range for stage "+s.Name, http.StatusBadRequest)
		return
	}

	log.Printf("Sending file %s to %s...", s.Trx[i].File, rc.name)

	// Read file and send it to the remote instance
	bytes, err := ioutil.ReadFile(s.Trx[i].File)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Write(bytes)
	log.Printf("Sent file %s to %s", s.Trx[i].File, rc.name)
}

func (a *API) run(w http.ResponseWriter, r *http.Request) {
	rc, get, ok := a.remote(w, r, false)
	if !ok {
		return // remote() wrote error response
	}

	if get {
		// GET /run: remote is waiting for signal to run previously booted stage
		if rc.state != runnable {
			w.WriteHeader(http.StatusPreconditionFailed)
			return
		}

		// Remote is waiting for next stage to run
		log.Printf("Remote %s waiting to start...", rc.name)
		for {
			select {
			case <-rc.stage.runChan:
			}
			if a.resetClient(rc, w, false) {
				return
			}
			break
		}
		rc.state = running           // advance remote state
		w.WriteHeader(http.StatusOK) // @todo handler error
		log.Printf("Started remote %s on stage %s\n", rc.name, rc.stage.cfg.Name)
	} else {
		// POST ./run: remote is done running stage
		if rc.state != running {
			w.WriteHeader(http.StatusPreconditionFailed)
			return
		}

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
		rc.stage.doneChan <- ack{name: rc.name, err: remoteErr}
		rc.state = ready // advance remote state (ready to run another stage)
	}
}

func (a *API) stats(w http.ResponseWriter, r *http.Request) {
	rc, _, ok := a.remote(w, r, false)
	if !ok {
		return // remote() wrote error response
	}

	// Stats are sent only while running. If this error occurs, it might just be
	// a network issue that delayed stats sent earlier (before remote stopped running).
	// If it happens frequently, then it's probably a bug in Finch.
	if rc.state != running {
		w.WriteHeader(http.StatusPreconditionFailed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("error reading error from remote: %s", err)
		return
	}
	r.Body.Close()
	w.WriteHeader(http.StatusOK)

	var s stats.Instance
	if err := json.Unmarshal(body, &s); err != nil {
		log.Printf("Invalid stats from %s: %s", rc.name, err)
		return
	}

	if rc.stage.stats != nil {
		rc.stage.stats.Recv(s)
	}
}

func (a *API) stop(w http.ResponseWriter, r *http.Request) {
	rc, get, ok := a.remote(w, r, false)
	if !ok {
		return // remote() wrote error response
	}

	if get {
		// GET /stop: remote asking "Keep running?" If stage reset, then remote()
		// already wrote error response to reset client.
		w.WriteHeader(http.StatusOK)
	} else {
		// POST /stop: remote is disconnecting
		delete(rc.stage.remotes, rc.name)
		w.WriteHeader(http.StatusOK)
		log.Printf("Remote %s disconnected", rc.name)
	}
}

// --------------------------------------------------------------------------

func (a *API) remote(w http.ResponseWriter, r *http.Request, boot bool) (*remote, bool, bool) {
	finch.Debug("%v", r)

	// GET or POST
	get := true
	switch r.Method {
	case http.MethodGet: // allowed
	case http.MethodPost: // allowed
		get = false
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		return nil, false, false
	}

	// ?name=...
	q := r.URL.Query()
	if len(q) == 0 {
		http.Error(w, "missing URL query: ?name=...", http.StatusBadRequest)
		return nil, false, false
	}
	vals, ok := q["name"]
	if !ok {
		http.Error(w, "missing name param in URL query: ?name=...", http.StatusBadRequest)
		return nil, false, false
	}
	if len(vals) == 0 {
		http.Error(w, "name param has no value, expected instance name", http.StatusBadRequest)
		return nil, false, false
	}
	name := clean(vals[0])

	vals, ok = q["stage-id"]
	if !ok {
		http.Error(w, "missing stage-id param in URL query: ?stage-id=...", http.StatusBadRequest)
		return nil, false, false
	}
	if len(vals) == 0 {
		http.Error(w, "stage-id param has no value", http.StatusBadRequest)
		return nil, false, false
	}
	sid := clean(vals[0])

	a.Lock()
	defer a.Unlock()
	rc, ok := a.stage.remotes[name]
	if ok {
		if rc.stage.reset {
			a.resetClient(rc, w, true)
			return nil, false, false
		}
		return rc, get, true // success
	}

	// New remote allowed only GET /boot
	if get && boot {
		rc = &remote{
			name:  name,
			state: ready,
		}
		a.stage.remotes[name] = rc // @todo remove, done in boot handler?
		return rc, get, true       // success (new remote)
	} else {
		// If not GET /boot (i.e. for all other requests), reset client if:
		//   - There's no active stage, or
		//   - Client's stage ID (sid) doesn't match the active stage
		a.Lock()
		stage := a.stage
		a.Unlock()
		if stage != nil || sid != stage.sid {
			a.resetClient(rc, w, true)
			return nil, false, false
		}
	}

	log.Printf("Unknown remote: %s", name)
	w.WriteHeader(http.StatusUnauthorized) // @todo reset
	return nil, false, false
}

// clean removes \n\r to avoid code scanning alert "Log entries created from user input".
func clean(s string) string {
	c := strings.Replace(s, "\n", "", -1)
	return strings.Replace(c, "\r", "", -1)
}
