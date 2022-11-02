// Copyright 2022 Block, Inc.

package client

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"runtime"
	"time"

	"github.com/square/finch"
	"github.com/square/finch/stats"
	"github.com/square/finch/workload"
)

// Client executes SQL statements.
type Client struct {
	ClientNo   int
	Name       string
	DB         *sql.DB
	Statements []workload.Statement
	Stats      *stats.Stats
	Runtime    time.Duration
	Iterations uint64
	QPS        <-chan bool
	TPS        <-chan bool
	DoneChan   chan error
	// --
	ps   []*sql.Stmt
	data [][]interface{}
	conn *sql.Conn
}

func (c *Client) connect(ctx context.Context, err error) bool {
	if c.conn != nil {
		c.conn.Close()
	}
	t0 := time.Now()
	if err != nil {
		select {
		case <-ctx.Done():
			return false // stop running; client is done
		default:
		}
		log.Printf("Client %s connection error: %s", err)
	}

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
				log.Printf("Client %d reconnected in %s", time.Now().Sub(t0))
				return true // success; keep running
			}
		}
		if i%10 == 0 {
			log.Printf("Client %d error reconnecting: %s (retrying)", err)
		}
		time.Sleep(1 * time.Second)
	}

	log.Printf("Client %d failed to reconnect to MySQL; stopping early", c.ClientNo)
	return false // cannot reconnect; stop running
}

func (c *Client) Prepare() error {
	c.connect(context.Background(), nil)

	c.ps = make([]*sql.Stmt, len(c.Statements))
	c.data = make([][]interface{}, len(c.Statements))
	for i, s := range c.Statements {
		if s.Prepare && !s.Defer {
			var err error
			c.ps[i], err = c.conn.PrepareContext(context.Background(), s.Query)
			if err != nil {
				return fmt.Errorf("prepare %s: %w", s.Query, err)
			}
		}
		if s.Values > 0 {
			c.data[i] = make([]interface{}, s.Values)
		}
	}
	return nil
}

func (c *Client) Run(ctxStage context.Context) {
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
		c.DoneChan <- err
	}()

	// @todo 1 shared ctxRun for client group
	var ctxRun context.Context
	var cancel context.CancelFunc
	if c.Runtime > 0 {
		ctxRun, cancel = context.WithDeadline(ctxStage, time.Now().Add(c.Runtime))
		defer cancel() // this client
	} else {
		ctxRun = ctxStage
	}

	c.connect(ctxRun, nil)

	// Deferred prepare
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

	finch.Debug("run %s: %d stmts, runtime %s, iters %d", c.Name, len(c.Statements), c.Runtime, c.Iterations)

	c.Stats.Reset() // sets begin=NOW()

	//
	// CRITICAL LOOP: no debug or superfluous function calls
	//
	var rows *sql.Rows
	var res sql.Result
	var t time.Time
	var iterNo uint64
ITER:
	for c.Iterations == 0 || iterNo < c.Iterations {
		iterNo += 1
		for i := range c.Statements {
			// Idle time
			if c.Statements[i].Idle != 0 {
				time.Sleep(c.Statements[i].Idle)
				continue
			}

			// Generate new data values for this query. A single data generator
			// can return multiple values, so d makes copy() append, else copy()
			// would start at [0:] each time
			d := 0
			for j := range c.Statements[i].Data {
				d += copy(c.data[i][d:], c.Statements[i].Data[j].Values())

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
					rows, err = c.ps[i].QueryContext(ctxRun, c.data[i]...)
				} else {
					rows, err = c.conn.QueryContext(ctxRun, fmt.Sprintf(c.Statements[i].Query, c.data[i]...))
				}
				c.Stats.Record(time.Now().Sub(t).Microseconds())
				if err != nil {
					err = fmt.Errorf("querying %s: %w", c.Statements[i].Query, err)
					if !c.connect(ctxRun, err) {
						return
					}
					continue ITER
				}
				if c.Statements[i].Columns == nil {
					for rows.Next() {
					} // Discard rows
					// @todo can we just rows.Close?
				} else {
					for rows.Next() {
						if err = rows.Scan(c.Statements[i].Columns...); err != nil {
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
					res, err = c.ps[i].ExecContext(ctxRun, c.data[i]...)
				} else {
					res, err = c.conn.ExecContext(ctxRun, fmt.Sprintf(c.Statements[i].Query, c.data[i]...))
				}
				c.Stats.Record(time.Now().Sub(t).Microseconds())
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
				if c.Statements[i].InsertId != nil {
					var n int64
					n, err = res.LastInsertId()
					if err != nil {
						err = fmt.Errorf("get last insert ID: %s", err)
						return
					}
					c.Statements[i].InsertId.Scan(n)
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
