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

// Client executes SQL statements.
type Client struct {
	ExecGroup        uint
	RunLevel         finch.RunLevel
	Statements       []*trx.Statement
	Data             []trx.Data
	Stats            []*stats.Trx `deep:"-"`
	TrxBoundary      []byte       `deep:"-"`
	DB               *sql.DB      `deep:"-"`
	IterExecGroup    uint32
	IterExecGroupPtr *uint32
	IterClients      uint32
	IterClientsPtr   *uint32
	Iter             uint
	QPS              <-chan bool
	TPS              <-chan bool
	DoneChan         chan *Client
	Error            error
	// --
	ps     []*sql.Stmt
	values [][]interface{}
	conn   *sql.Conn
}

func (c *Client) Init() error {
	c.ps = make([]*sql.Stmt, len(c.Statements))
	c.values = make([][]interface{}, len(c.Statements))
	for i, s := range c.Statements {
		if len(s.Inputs) > 0 {
			c.values[i] = make([]interface{}, len(s.Inputs))
		}
	}
	return nil
}

func (c *Client) Connect(ctx context.Context, cerr error) bool {
	if c.conn != nil {
		c.conn.Close()
	}
	if cerr != nil {
		log.Printf("Client %s connection error: %s", c.RunLevel.ClientId(), cerr)
	}
	t0 := time.Now()
	for i := 0; i < 100; i++ {
		if ctx.Err() != nil {
			return false
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

func (c *Client) Prepare(ctxExec context.Context) error {
	for i, s := range c.Statements {
		if !s.Prepare {
			continue
		}
		if c.ps[i] != nil {
			continue // prepare multi
		}
		var err error
		c.ps[i], err = c.conn.PrepareContext(ctxExec, s.Query)
		if err != nil {
			return fmt.Errorf("prepare %s: %w", s.Query, err)
		}

		// If s.PrepareMulti = 3, it means this ps should be used for 3 statments
		// including this one, so copy it into the next 2 statements. If = 0, this
		// loop doesn't run becuase j = 1; j < 0 is immediately false.
		for j := 1; j < s.PrepareMulti; j++ {
			c.ps[i+j] = c.ps[i]
		}
	}
	return nil
}

func (c *Client) Run(ctxExec context.Context) {
	finch.Debug("run client %s: %d stmts, runtime %s, iters %d/%d/%d", c.RunLevel.ClientId(), len(c.Statements), c.IterExecGroup, c.IterClients, c.Iter)

	var err error
	var runtimeElapsed bool

	defer func() {
		if r := recover(); r != nil {
			b := make([]byte, 4096)
			n := runtime.Stack(b, false)
			err = fmt.Errorf("PANIC: %v\n%s", r, string(b[0:n]))
		}
		// Must close prepared statments before returning to DoneChan to ensure
		// Finch doesn't exit before we're closed them, else we'll leak them in
		// MySQL.
		for i := range c.ps {
			if c.ps[i] == nil {
				continue
			}
			c.ps[i].Close()
		}
		// If the runtime elapses, err can be a "context canceled" error,
		// which we can ignore.
		if !runtimeElapsed {
			c.Error = err
		}
		c.DoneChan <- c
	}()

	c.Connect(ctxExec, nil)

	if err = c.Prepare(ctxExec); err != nil {
		return
	}

	var rows *sql.Rows
	var res sql.Result
	var t time.Time

	cnt := finch.ExecCount{}
	cnt[finch.STAGE] = finch.StageNumber[c.RunLevel.Stage]
	cnt[finch.EXEC_GROUP] = c.RunLevel.ExecGroup
	cnt[finch.CLIENT_GROUP] = c.RunLevel.ClientGroup

	// trxNo indexes into c.Stats and resets to 0 on each iteration. Remember:
	// these are finch trx (files), not MySQL trx, so trx boundaries mark the
	// beginning and end of a finch trx (file). User is expected to make finch
	// trx boundaries meaningful.
	trxNo := -1

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
		trxNo = -1

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
				trxNo += 1
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
				//
				// SELECT
				//
				t = time.Now()
				if c.ps[i] != nil {
					rows, err = c.ps[i].QueryContext(ctxExec, c.values[i]...)
				} else {
					rows, err = c.conn.QueryContext(ctxExec, fmt.Sprintf(c.Statements[i].Query, c.values[i]...))
				}
				if c.Stats[trxNo] != nil {
					c.Stats[trxNo].Record(stats.READ, time.Now().Sub(t).Microseconds())
				}
				if err != nil {
					if ctxExec.Err() != nil {
						err = nil
						return
					}
					err = fmt.Errorf("querying %s: %w", c.Statements[i].Query, err)
					if !c.Connect(ctxExec, err) {
						return
					}
					continue ITER
				}
				if c.Data[i].Outputs != nil {
					for rows.Next() {
						if err = rows.Scan(c.Data[i].Outputs...); err != nil {
							rows.Close()
							err = fmt.Errorf("scanning result set of %s: %w", c.Statements[i].Query, err)
							if !c.Connect(ctxExec, err) {
								return
							}
							continue ITER
						}
					}
				}
				rows.Close()
			} else {
				//
				// Write or query without result set (e.g. BEGIN, SET, etc.)
				//
				if c.Statements[i].Limit != nil { // limit rows -------------
					if !c.Statements[i].Limit.More(c.conn) {
						return // chan closed = no more writes
					}
				}
				t = time.Now()
				if c.ps[i] != nil { // exec ---------------------------------
					res, err = c.ps[i].ExecContext(ctxExec, c.values[i]...)
				} else {
					res, err = c.conn.ExecContext(ctxExec, fmt.Sprintf(c.Statements[i].Query, c.values[i]...))
				}
				if c.Stats[trxNo] != nil { // record stats ------------------
					switch {
					case c.Statements[i].Write:
						c.Stats[trxNo].Record(stats.WRITE, time.Now().Sub(t).Microseconds())
					case c.Statements[i].Commit:
						c.Stats[trxNo].Record(stats.COMMIT, time.Now().Sub(t).Microseconds())
					default:
						// BEGIN, SET, and other statements that aren't reads or writes
						// but count and response time will be included in total
						c.Stats[trxNo].Record(stats.TOTAL, time.Now().Sub(t).Microseconds())
					}
				}
				if err != nil { // handle err, if any -----------------------
					if ctxExec.Err() != nil {
						err = nil
						return
					}
					err = fmt.Errorf("executing %s: %w", c.Statements[i].Query, err)
					if !c.Connect(ctxExec, err) {
						return
					}
					continue ITER
				}
				if c.Statements[i].Limit != nil { // limit rows -------------
					n, _ := res.RowsAffected()
					c.Statements[i].Limit.Affected(n)
				}
				if c.Data[i].InsertId != nil { // insert ID -----------------
					id, _ := res.LastInsertId()
					c.Data[i].InsertId.Scan(id)
				}
			} // execute
		} // statements
	} // iterations
}
