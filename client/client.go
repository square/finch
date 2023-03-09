// Copyright 2022 Block, Inc.

package client

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/square/finch"
	"github.com/square/finch/stats"
	"github.com/square/finch/trx"
)

type Group struct {
	ExecGroup string
	Trx       []string
}

// Client executes SQL statements.
type Client struct {
	RunLevel         finch.RunLevel
	Statements       []*trx.Statement
	Data             []trx.Data
	Stats            *stats.Stats `deep:"-"`
	TrxBoundary      []byte       `deep:"-"`
	DB               *sql.DB      `deep:"-"`
	Runtime          time.Duration
	IterExecGroup    uint32
	IterExecGroupPtr *uint32
	IterClients      uint32
	IterClientsPtr   *uint32
	Iter             uint
	QPS              <-chan bool
	TPS              <-chan bool
	DoneChan         chan error
	// --
	ps     []*sql.Stmt
	values [][]interface{}
	conn   *sql.Conn
}

func (c *Client) connect(ctx context.Context, cerr error) bool {
	if c.conn != nil {
		c.conn.Close()
	}

	// If there's a caller error, report it and (later) report when we reconnect.
	// Else (if no cerr), caller is just connecting first time, so don't log unless
	// connecting fails.
	if cerr != nil {
		// @todo return false if "Error 1054: Unknown column"
		select {
		case <-ctx.Done():
			return false // stop running; client is done
		default:
		}
		log.Printf("Client %s connection error: %s", c.RunLevel.ClientId(), cerr)
	}

	t0 := time.Now()
	for i := 0; i < 100; i++ {
		// Runtime elapsed?
		select {
		case <-ctx.Done():
			return false
		default:
		}
		var err error
		c.conn, err = c.DB.Conn(ctx)
		if err == nil {
			err = c.conn.PingContext(ctx)
			if err == nil {
				if cerr != nil {
					log.Printf("Client %s reconnected in %s", c.RunLevel.ClientId(), time.Now().Sub(t0))
				}
				return true // success; keep running
			}
		}
		if i%10 == 0 {
			log.Printf("Client %s error reconnecting: %s (retrying)", c.RunLevel.ClientId(), err)
		}
		time.Sleep(1 * time.Second)
	}
	log.Printf("Client %s failed to reconnect to MySQL; stopping early", c.RunLevel.ClientId())
	return false // cannot reconnect; stop running
}

func (c *Client) Prepare() error {
	c.connect(context.Background(), nil)

	c.ps = make([]*sql.Stmt, len(c.Statements))
	c.values = make([][]interface{}, len(c.Statements))
	for i, s := range c.Statements {
		if s.Prepare && !s.Defer {
			var err error
			c.ps[i], err = c.conn.PrepareContext(context.Background(), s.Query)
			if err != nil {
				return fmt.Errorf("prepare %s: %w", s.Query, err)
			}
		}
		if len(s.Inputs) > 0 {
			c.values[i] = make([]interface{}, len(s.Inputs))
		}
	}
	return nil
}

func (c *Client) Run(ctxStage context.Context) {
	finch.Debug("run client %s: %d stmts, runtime %s, iters %d/%d/%d", c.RunLevel.ClientId(), len(c.Statements), c.Runtime, c.IterExecGroup, c.IterClients, c.Iter)

	// @todo 1 shared ctxRun for client group
	var ctxRun context.Context
	var cancel context.CancelFunc
	if c.Runtime > 0 {
		ctxRun, cancel = context.WithDeadline(ctxStage, time.Now().Add(c.Runtime))
		defer cancel() // this client
	} else {
		ctxRun = ctxStage
	}

	var err error
	defer func() {
		if r := recover(); r != nil {
			b := make([]byte, 4096)
			n := runtime.Stack(b, false)
			err = fmt.Errorf("PANIC: %v\n%s", r, string(b[0:n]))
		}
		for i := range c.ps {
			if c.ps[i] == nil {
				continue
			}
			c.ps[i].Close()
		}
		select {
		case <-ctxRun.Done():
			c.DoneChan <- nil // no error, just runtime elapsed
		default:
		}

		c.DoneChan <- err
	}()

	c.connect(ctxRun, nil)

	// Deferred prepare, rquired when db/table does exist yet
	for i, s := range c.Statements {
		if !s.Prepare || c.ps[i] != nil {
			continue
		}
		finch.Debug("deferred prepare: %s", s.Query)
		c.ps[i], err = c.conn.PrepareContext(ctxRun, s.Query)
		if err != nil {
			err = fmt.Errorf("prepare (deferred) %s: %w", s.Query, err)
			return
		}
	}

	var rows *sql.Rows
	var res sql.Result
	var t time.Time

	cnt := finch.ExecCount{}
	cnt[finch.STAGE] = finch.StageNumber[c.RunLevel.Stage]
	cnt[finch.EXEC_GROUP] = c.RunLevel.ExecGroup
	cnt[finch.CLIENT_GROUP] = c.RunLevel.ClientGroup

	if c.Stats != nil {
		c.Stats.Reset() // sets begin=NOW()
	}

	//
	// CRITICAL LOOP: no debug or superfluous function calls
	//
ITER:
	for {
		if c.IterExecGroup > 0 && atomic.AddUint32(c.IterExecGroupPtr, 1) > c.IterExecGroup {
			return
		}
		if c.IterClients > 0 && atomic.AddUint32(c.IterClientsPtr, 1) > c.IterClients {
			return
		}
		if c.Iter > 0 && cnt[finch.ITER] == c.Iter {
			return
		}
		cnt[finch.ITER] += 1

		for i := range c.Statements {
			// Idle time
			if c.Statements[i].Idle != 0 {
				time.Sleep(c.Statements[i].Idle)
				continue
			}

			cnt[finch.QUERY] += 1

			// Is this query the start of a new (finch) trx file? This is not
			// a MySQL trx (either BEGIN or implicit). It marks finch trx scope
			// "trx" is a trx file in the config assigned to this client.
			if c.Data[i].TrxBoundary&finch.TRX_BEGIN == 1 {
				cnt[finch.TRX] += 1
			}

			// Generate new data values for this query. A single data generator
			// can return multiple values, so d makes copy() append, else copy()
			// would start at [0:] each time
			d := 0
			for _, g := range c.Data[i].Inputs {
				d += copy(c.values[i][d:], g.Values(cnt))
			}

			// If BEGIN, check TPS rate limiter
			if c.TPS != nil && c.Statements[i].Begin {
				<-c.TPS
			}

			// If query, check QPS
			if c.QPS != nil {
				<-c.QPS
			}

			if c.Statements[i].ResultSet {
				// SELECT
				t = time.Now()
				if c.ps[i] != nil {
					rows, err = c.ps[i].QueryContext(ctxRun, c.values[i]...)
				} else {
					rows, err = c.conn.QueryContext(ctxRun, fmt.Sprintf(c.Statements[i].Query, c.values[i]...))
				}
				if c.Stats != nil {
					c.Stats.Record(stats.READ, time.Now().Sub(t).Microseconds())
				}
				if err != nil {
					err = fmt.Errorf("querying %s: %w", c.Statements[i].Query, err)
					if !c.connect(ctxRun, err) {
						return
					}
					continue ITER
				}
				if c.Data[i].Outputs != nil {
					for rows.Next() {
						if err = rows.Scan(c.Data[i].Outputs...); err != nil {
							rows.Close()
							err = fmt.Errorf("scanning result set of %s: %w", c.Statements[i].Query, err)
							if !c.connect(ctxRun, err) {
								return
							}
							continue ITER
						}
					}
				}
				rows.Close()
			} else {
				// Write or query without result set (e.g. BEGIN, SET, etc.)
				if c.Statements[i].Limit != nil {
					if !c.Statements[i].Limit.More(c.conn) {
						return // chan closed = no more writes
					}
				}
				t = time.Now()
				if c.ps[i] != nil {
					res, err = c.ps[i].ExecContext(ctxRun, c.values[i]...)
				} else {
					res, err = c.conn.ExecContext(ctxRun, fmt.Sprintf(c.Statements[i].Query, c.values[i]...))
				}
				if c.Stats != nil {
					switch {
					case c.Statements[i].Write:
						c.Stats.Record(stats.WRITE, time.Now().Sub(t).Microseconds())
					case c.Statements[i].Commit:
						c.Stats.Record(stats.COMMIT, time.Now().Sub(t).Microseconds())
					default:
						// BEGIN, SET, and other statements that aren't reads or writes
						// but count and response time will be included in total
						c.Stats.Record(stats.TOTAL, time.Now().Sub(t).Microseconds())
					}
				}
				if err != nil {
					err = fmt.Errorf("executing %s: %w", c.Statements[i].Query, err)
					if !c.connect(ctxRun, err) {
						return
					}
					continue ITER
				}
				if c.Statements[i].Limit != nil {
					n, _ := res.RowsAffected()
					c.Statements[i].Limit.Affected(n)
				}
				if c.Data[i].InsertId != nil {
					id, _ := res.LastInsertId()
					c.Data[i].InsertId.Scan(id)
				}
			}
		} // statements

		// Runtime elapsed?
		select {
		case <-ctxRun.Done():
			return
		default:
		}

	} // iterations
}
