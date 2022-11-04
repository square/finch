// Copyright 2022 Block, Inc.

package client

import (
	"fmt"
	"time"

	"github.com/square/finch"
	"github.com/square/finch/config"
	"github.com/square/finch/dbconn"
	"github.com/square/finch/limit"
	"github.com/square/finch/stats"
	"github.com/square/finch/workload"
)

type AllocateArgs struct {
	Stage          string
	ClientGroups   []config.ClientGroup    // config.stage.clients
	AutoAllocation bool                    // !config.stage.disable-auto-allocation
	Trx            map[string]workload.Trx // trx name => statements
	WorkloadOrder  []string                // trx name (in worload order)
	Sequence       string
	DoneChan       chan error // Stage.doneChan
	StageQPS       limit.Rate
	StageTPS       limit.Rate
}

func Allocate(a AllocateArgs) ([][]*Client, error) {
	if a.AutoAllocation {
		if len(a.ClientGroups) == 0 {
			// Full auto alloc
			if a.Stage == "setup" || a.Stage == "cleanup" {
				finch.Debug("auto client group: %s", a.Stage)
				a.ClientGroups = make([]config.ClientGroup, len(a.WorkloadOrder))
				for i, name := range a.WorkloadOrder {
					a.ClientGroups[i] = config.ClientGroup{
						N:            1,
						Iterations:   1,
						Transactions: []string{name},
					}

					dataLimit := false
					for i := range a.Trx[name].Statements {
						if a.Trx[name].Statements[i].Limit != nil {
							dataLimit = true
							break
						}
					}
					if dataLimit {
						finch.Debug("trx %s has data limit; iter=0", name)
						a.ClientGroups[i].Iterations = 0
					}
					finch.Debug("auto alloc %s: %+v", a.Stage, a.ClientGroups[i])
				}
			} else {
				// Warmup and benchmark
				finch.Debug("auto warmup/benchmark client group")
				a.ClientGroups = []config.ClientGroup{
					{
						N:            1,
						Transactions: a.WorkloadOrder,
					},
				}
				finch.Debug("auto alloc %s: %+v", a.Stage, a.ClientGroups[0])
			}
		} else {
			// partial auto alloc (fill in the gaps)
			// Ensure each trx has a client group
			// If one doesn't, give it a default client group
		WORKLOAD:
			for _, name := range a.WorkloadOrder {
				for _, cg := range a.ClientGroups {
					for i := range cg.Transactions {
						if cg.Transactions[i] == name {
							continue WORKLOAD
						}
					}
				}
				// Not assigned, so assign it
				cg := config.ClientGroup{
					N:            1,
					Transactions: []string{name},
				}
				if a.Stage == "setup" || a.Stage == "cleanup" {
					cg.Iterations = 1
				}
				finch.Debug("auto alloc %s: %+v", a.Stage, cg)
				a.ClientGroups = append(a.ClientGroups, cg)
			}
		}
	}

	qpsLimits := make([]limit.Rate, len(a.ClientGroups))
	tpsLimits := make([]limit.Rate, len(a.ClientGroups))
	for i, cg := range a.ClientGroups {
		if len(cg.Transactions) == 0 {
			return nil, fmt.Errorf("client group %d: no transactions assigned", i+1)
		}
		qpsLimits[i] = limit.And(a.StageQPS, limit.NewRate(cg.QPS))
		tpsLimits[i] = limit.And(a.StageTPS, limit.NewRate(cg.TPS))
	}

	var clients [][]*Client // [sequence]   [clients]
	var order [][]int       // [clients[i]] [ClientGroups[j]...]

	switch a.Sequence {
	case "workload":
		finch.Debug("sequence by workload")
		clients = make([][]*Client, len(a.WorkloadOrder))
		order = make([][]int, len(a.WorkloadOrder))
		for i, trxName := range a.WorkloadOrder {
			order[i] = []int{}
			for j, cg := range a.ClientGroups {
				if cg.Name() != trxName {
					continue
				}
				order[i] = append(order[i], j)
			}
		}
	case "clients":
		finch.Debug("sequence by clients")
		clients = make([][]*Client, len(a.ClientGroups))
		order = make([][]int, len(a.ClientGroups))
		for j := range a.ClientGroups {
			order[j] = []int{j}
		}
	default:
		finch.Debug("shotgun finch")
		clients = make([][]*Client, 1)
		order = make([][]int, 1)
		order[0] = make([]int, len(a.ClientGroups))
		for j := range a.ClientGroups {
			order[0][j] = j
		}
	}

	clientNo := 1
	for i := range order {
		clients[i] = []*Client{}
		for _, j := range order[i] {
			finch.Debug("client group %d: %#v", j, a.ClientGroups[j])
			for n := uint(0); n < a.ClientGroups[j].N; n++ {
				finch.Debug("  client %d seq %d: %dx %s (%d)", clientNo, i, a.ClientGroups[j].N, a.ClientGroups[j].Name(), j)
				c, err := allocClient(a, a.ClientGroups[j], qpsLimits[j], tpsLimits[j], clientNo)
				if err != nil {
					return nil, err
				}
				clients[i] = append(clients[i], c)
				clientNo++
			}
		}
	}

	return clients, nil
}

// allocClient allocates one client in a group.
func allocClient(a AllocateArgs, cg config.ClientGroup, qps, tps limit.Rate, clientNo int) (*Client, error) {
	// Make a new sql.DB and sql.Conn for this client. Yes, sql.DB are meant
	// to be shared, but this is a benchmark tool and each client is meant to be
	// isolated. So we duplicate per client so there's zero chance of one client
	// affecting another via a shared sql.DB. The connection to MySQL was already
	// tested in compute/Instance.Boot, so these should not error.
	db, _, err := dbconn.Make()
	if err != nil {
		return nil, err
	}

	// Copy all statements from all transactions assigned to this client,
	// which may be only a subset of the workload. This must be a deep copy
	// because s.Data = []data.Generator, i.e. don't introduce bug due to
	// shallow copy of Go slice, which would give every statement in every
	// client the same data generator (and therefore likely cause race conditions
	// on data value generation). See Statement.Copy() in ../workload/.
	statements := []workload.Statement{}
	for _, name := range cg.Transactions {
		statements = append(statements, a.Trx[name].ClientCopy(clientNo)...)
	}
	finch.Debug("    %d statments", len(statements))
	for i := range statements {
		for j := range statements[i].Data {
			finch.Debug("    %s", statements[i].Data[j].Id())
		}
	}

	runtime, _ := time.ParseDuration(cg.Runtime) // already validated

	c := &Client{
		ClientNo:   clientNo,
		Name:       cg.Name(),
		DB:         db,
		Statements: statements,
		Stats:      stats.NewStats(),
		Iterations: cg.Iterations,
		Runtime:    runtime,
		DoneChan:   a.DoneChan,
	}
	if qps != nil {
		c.QPS = qps.Allow()
	}
	if tps != nil {
		c.TPS = tps.Allow()
	}
	return c, nil
}
