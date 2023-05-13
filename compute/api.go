package compute

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
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
	*sync.Mutex
	httpServer *http.Server
	stage      *stageMeta // current stage
	prev       map[string]string
}

const (
	ready byte = iota
	booting
	runnable
	running
)

type stageMeta struct {
	*sync.Mutex
	cfg      config.Stage
	nRemotes uint
	bootChan chan ack         // 1. <-client after booting stage
	runChan  chan struct{}    // 2. server closes to signal clients to run
	doneChan chan ack         // 3. <-client after running stage
	stats    *stats.Collector // receives stats from clients while running
	booted   bool
	done     bool
	clients  map[string]*client
}

type client struct {
	name  string
	stage *stageMeta
	state byte
}

func NewAPI(addr string) *API {
	a := &API{
		Mutex: &sync.Mutex{},
	}

	// HTTP server that client instances calls
	mux := http.NewServeMux()
	mux.HandleFunc("/boot", a.boot)
	mux.HandleFunc("/file", a.file)
	mux.HandleFunc("/run", a.run)
	mux.HandleFunc("/stats", a.stats)
	mux.HandleFunc("/ping", a.ping)
	a.httpServer = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	// Make sure we can bind to addr:port. ListenAndServe will return an error
	// but it's run in a goroutine so that error will occur async to the boot,
	// which is a poor experience: failure a millisecond after boot. This makes
	// it sync, so nothing boots if it fails. ListenAndServe might still fail
	// for other reasons, but that's unlikely, so this check is good enough.
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatal(err)
	}
	ln.Close()
	go func() {
		if err := a.httpServer.ListenAndServe(); err != nil {
			log.Fatal(err)
		}
		log.Println("Listening on", addr)
	}()
	return a
}

// ServeHTTP implements the http.HandlerFunc interface.
func (a *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.httpServer.Handler.ServeHTTP(w, r)
}

func (a *API) Stage(newStage *stageMeta) error {
	if newStage != nil {
		finch.Debug("new stage %s (%s)", newStage.cfg.Name, newStage.cfg.Id)
	}

	// If there's no current stage, set new one and done
	a.Lock()
	if a.stage == nil {
		a.stage = newStage
		a.Unlock()
		return nil
	}

	// Stop current (old) stage before setting new stage.
	oldStage := a.stage
	a.Unlock()

	// Signal clients that stage has stopped early
	finch.Debug("stop old stage %s (%s)", oldStage.cfg.Name, oldStage.cfg.Id)
	oldStage.Lock()
	oldStage.done = true
	if oldStage.cfg.Test {
		close(oldStage.runChan)
	}
	oldStage.Unlock()

	// Wait for clients to check in (GET /run), be signaled that stage.done=true,
	// send final stats, then call POST /run to terminate
	timeout := time.After(3 * time.Second)
	for {
		time.Sleep(100 * time.Millisecond)
		select {
		case <-timeout:
			finch.Debug("timeout waiting for clients to reset")
			break
		default:
		}
		oldStage.Lock()
		n := len(oldStage.clients)
		oldStage.Unlock()
		if n == 0 {
			break
		}
	}

	oldStage.Lock()
	if len(oldStage.clients) > 0 {
		log.Printf("%d clients did not stop, ignoring (stats will be lost): %v", len(oldStage.clients), oldStage.clients)
	}
	oldStage.Unlock()

	// Set new stage now that old stage has stopped
	a.Lock()
	a.stage = newStage
	a.Unlock()

	return nil
}

func (a *API) boot(w http.ResponseWriter, r *http.Request) {
	rc, get, ok := a.client(w, r, true) // true == allow new clients on GET /boot
	if !ok {
		return // client() wrote error response
	}

	if get {
		// GET /boot: client is booting, waiting to receive config.File
		if rc.state != ready {
			w.WriteHeader(http.StatusPreconditionFailed)
			return
		}

		// Wait until there's a stage that's not done booting (needs more instances)
		log.Printf("Remote %s ready to boot\n", rc.name)
		for {
			// Has server set a stage?
			a.Lock()
			stage := a.stage // copy ptr
			if stage == nil || stage.done {
				a.Unlock()
				goto RETRY // no stage
			}

			// Is the stage still booting (waiting for instances)?
			stage.Lock()
			if stage.booted || len(stage.clients) == int(stage.nRemotes) {
				stage.Unlock()
				a.Unlock()
				goto RETRY // stage is full
			}

			// Stage is ready and there's a space for this client
			stage.clients[rc.name] = rc
			rc.stage = stage
			rc.state = booting // advance client state

			// Unwind locks before sending stage config via HTTP in case net is slow
			stage.Unlock()
			a.Unlock()

			finch.Debug("assigned %s to stage %s (%s): %d of %d clients", rc.name, stage.cfg.Name, stage.cfg.Id,
				len(stage.clients), stage.nRemotes)
			json.NewEncoder(w).Encode(stage.cfg) // send stage config
			return

		RETRY:
			time.Sleep(200 * time.Millisecond)
		}
	} else {
		// POST /boot: client is ack'ing previous GET /boot; body is error message, if any
		if rc.state != booting {
			w.WriteHeader(http.StatusPreconditionFailed)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			log.Printf("error reading error from client: %s", err)
			return
		}
		r.Body.Close()
		w.WriteHeader(http.StatusOK)

		// Remote might fail to boot. If that's the case, do not advance its state;
		// it should call GET /boot again to reset itself and try again.
		var clientErr error
		if string(body) != "" {
			// Don't advance state: client failed to boot, so it's not ready to run
			clientErr = fmt.Errorf("%s", string(body))
		} else {
			rc.state = runnable // advance client state (successful boot)
		}
		rc.stage.bootChan <- ack{name: rc.name, err: clientErr}

	}
}

func (a *API) file(w http.ResponseWriter, r *http.Request) {
	rc, _, ok := a.client(w, r, false)
	if !ok {
		return // client() wrote error response
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

	// Read file and send it to the client instance
	bytes, err := ioutil.ReadFile(s.Trx[i].File)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Write(bytes)
	log.Printf("Sent file %s to %s", s.Trx[i].File, rc.name)
}

func (a *API) run(w http.ResponseWriter, r *http.Request) {
	rc, get, ok := a.client(w, r, false)
	if !ok {
		return // client() wrote error response
	}

	if get {
		// GET /run: client is waiting for signal to run previously booted stage
		if rc.state != runnable {
			w.WriteHeader(http.StatusPreconditionFailed)
			return
		}

		// Remote is waiting for next stage to run
		log.Printf("Remote %s waiting to start...", rc.name)
		<-rc.stage.runChan // closed in Server.Run, or api.Stage if --test

		// If boot --test and there's a new stage, Server.Boot calls api.Stage
		// which will stop the old stage and trigger this block.
		rc.stage.Lock()
		if rc.stage.done {
			delete(rc.stage.clients, rc.name)
			rc.stage.Unlock()
			w.WriteHeader(http.StatusResetContent) // reset
			return
		}
		rc.stage.Unlock()

		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte{0}); err != nil {
			log.Printf("Lost client %s on stage %s, but it will return\n", rc.name, rc.stage.cfg.Name)
			return
		}

		log.Printf("Started client %s on stage %s\n", rc.name, rc.stage.cfg.Name)
		rc.state = running // advance client state
	} else {
		// POST /run: client is done running stage
		if rc.state != running {
			w.WriteHeader(http.StatusPreconditionFailed)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			// Ignore error; it doesn't change fact that client is done
			log.Printf("Error reading error from client on POST /run, ignoring: %s", err)
		}
		r.Body.Close()
		w.WriteHeader(http.StatusOK)

		rc.stage.Lock()
		delete(rc.stage.clients, rc.name)
		rc.stage.Unlock()

		// Tell server client completed stage
		var clientErr error
		if string(body) != "" {
			clientErr = fmt.Errorf("%s", string(body))
		}
		rc.stage.doneChan <- ack{name: rc.name, err: clientErr}
		rc.state = ready // advance client state (ready to run another stage)
	}
}

func (a *API) stats(w http.ResponseWriter, r *http.Request) {
	rc, _, ok := a.client(w, r, false)
	if !ok {
		return // client() wrote error response
	}

	// Stats are sent only while running. If this error occurs, it might just be
	// a network issue that delayed stats sent earlier (before client stopped running).
	// If it happens frequently, then it's probably a bug in Finch.
	if rc.state != running {
		w.WriteHeader(http.StatusPreconditionFailed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("error reading error from client: %s", err)
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

func (a *API) ping(w http.ResponseWriter, r *http.Request) {
	rc, _, ok := a.client(w, r, false)
	if !ok {
		return // client() wrote error response
	}
	rc.stage.Lock()
	done := rc.stage.done
	rc.stage.Unlock()
	if done {
		log.Printf("Stage done, resetting %s", rc.name)
		w.WriteHeader(http.StatusResetContent) // reset
		return
	}
	w.WriteHeader(http.StatusOK) // keep running
}

// --------------------------------------------------------------------------

func (a *API) client(w http.ResponseWriter, r *http.Request, boot bool) (*client, bool, bool) {
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

	// Has server set a stage? Instances can connect before server is ready.
	if a.stage == nil {
		w.WriteHeader(http.StatusGone)
		return nil, false, false
	}

	a.stage.Lock()
	defer a.stage.Unlock()

	// Is instance assigned to the current stage?
	rc, ok := a.stage.clients[name]
	if !ok {

		// Instance not assigned to the stage, but that's ok if it's trying
		// to boot and join the stage.
		if get && boot {
			finch.Debug("new client")
			rc = &client{
				name:  name,
				state: ready,
			}
			// Do not add to stage.clients; that's done in boot() if this client
			// is assigned to the stage
			return rc, get, true // success (new client)
		}

		// Instance not assigned to stage and not booting, so it's out of sync
		log.Printf("Unknown client: %s", name)
		w.WriteHeader(http.StatusGone) // reset
		return nil, false, false
	}

	// Instance is assigned to the stage, but check stage ID to make sure a bad
	// network partition (or some other net delay/weirdness) hasn't caused a
	// _past_ query from the instance to finally reach us now after the stage
	// has changed.
	if !a.stage.done && a.stage.cfg.Id != sid {
		log.Printf("Wrong stage ID: %s: client %s != current %s", name, sid, a.stage.cfg.Id)
		w.WriteHeader(http.StatusGone) // reset
		return nil, false, false
	}

	return rc, get, true // success
}

// clean removes \n\r to avoid code scanning alert "Log entries created from user input".
func clean(s string) string {
	c := strings.Replace(s, "\n", "", -1)
	return strings.Replace(c, "\r", "", -1)
}
