// Copyright 2023 Block, Inc.

package workload

import (
	"fmt"
	"time"

	"github.com/square/finch"
	"github.com/square/finch/client"
	"github.com/square/finch/config"
	"github.com/square/finch/data"
	"github.com/square/finch/dbconn"
	"github.com/square/finch/limit"
	"github.com/square/finch/stats"
	"github.com/square/finch/trx"
)

// Allocator allocate a workload by using configured client groups (config.ClientGroup)
// to create runnable client groups (workload.ClientGroup) grouped into execution groups.
// It has two methods that must be called in order: Groups, then Clients. This is done
// once for each stage in Stage.Prepare.
//
// Allocator modifies Workload.
type Allocator struct {
	Stage    string
	TrxSet   *trx.Set             // config.stage.trx
	Workload []config.ClientGroup // config.stage.workload
	StageQPS limit.Rate           // config.stage.qps
	StageTPS limit.Rate           // config.stage.tps
	DoneChan chan *client.Client  // Stage.doneChan
}

// ClientGroup is a runnable group of clients created from a config.ClientGroup.
// It's the return type of Clients, grouped by the execution groups returned by
// Groups:
//
//	[]config.ClientGroup -> Groups -> Clients -> [][]workload.ClientGroup
type ClientGroup struct {
	Runtime time.Duration // used by Stage to create a single ctx for all clients in the group
	Clients []*client.Client
}

func (a *Allocator) Groups() ([][]int, error) {
	// Execution is always sequential by exec group, but by default
	// setup/cleanup are sequential, and warmup/benchmark are concurrent.
	// This default determines how to auto-alloc stuff below when needed.
	seq := a.Stage == finch.STAGE_SETUP || a.Stage == finch.STAGE_CLEANUP

	// Special case: no client groups specified (config.stage.workload=[]),
	// so auto-allocate the workload based on the stage default seq or not.
	if len(a.Workload) == 0 {
		if seq {
			// 1 client group per trx executed sequentially (default setup workload)
			a.Workload = make([]config.ClientGroup, len(a.TrxSet.Order))
			for i, trxName := range a.TrxSet.Order {
				a.Workload[i] = config.ClientGroup{
					Clients: 1,
					Iter:    1,
					Trx:     []string{trxName},
				}
				dataLimit := false
				for i := range a.TrxSet.Statements[trxName] {
					if a.TrxSet.Statements[trxName][i].Limit != nil {
						dataLimit = true
						break
					}
				}
				if dataLimit {
					finch.Debug("trx %s has data limit; iter=0", trxName)
					a.Workload[i].Iter = 0
				}
			}
		} else {
			// 1 exec for all trx executed concurrently (default benchmark workload)
			a.Workload = []config.ClientGroup{
				{
					Clients: 1,
					Trx:     a.TrxSet.Order,
				},
			}
		}
	}

	// Fill in missing client group names. Names are required because they determine
	// exec groups in the follow code block.
	autoNo := 1
	prevExplicit := false
	for i := range a.Workload {
		if len(a.Workload[i].Trx) == 0 {
			a.Workload[i].Trx = a.TrxSet.Order
		}
		if a.Workload[i].Name == "" {
			if seq {
				a.Workload[i].Name = fmt.Sprintf("auto%d", i+1)
			} else {
				if prevExplicit {
					autoNo += 1
				}
				a.Workload[i].Name = fmt.Sprintf("auto%d", autoNo)
			}
			prevExplicit = false
		} else {
			prevExplicit = true
		}
	}

	// Group client groups by name to form exec groups.
	groups := [][]int{}
	groupNo := -1
	name := ""
	for i := range a.Workload {
		if name != a.Workload[i].Name {
			groups = append(groups, []int{})
			groupNo += 1
			name = a.Workload[i].Name
		}
		groups[groupNo] = append(groups[groupNo], i)
	}
	for i := range a.Workload {
		finch.Debug("%3d exec group: %+v", i, a.Workload[i])
	}
	finch.Debug("exec groups: %+v", groups)

	return groups, nil
}

func (a *Allocator) Clients(groups [][]int) ([][]ClientGroup, error) {
	clients := make([][]ClientGroup, len(groups))
	runlevel := &finch.RunLevel{
		Stage:         a.Stage,
		ExecGroup:     0,
		ExecGroupName: "",
		ClientGroup:   0,
		Client:        0,
		Trx:           0,
		TrxName:       "",
		Query:         0,
	}

	for egNo := range groups {
		runlevel.ExecGroup = uint(egNo + 1)

		cgFirst := a.Workload[groups[egNo][0]]
		runlevel.ExecGroupName = cgFirst.Name

		execGroupQPS := limit.And(a.StageQPS, limit.NewRate(cgFirst.QPSExecGroup))
		execGroupTPS := limit.And(a.StageTPS, limit.NewRate(cgFirst.TPSExecGroup))

		clients[egNo] = make([]ClientGroup, len(groups[egNo]))

		var execGroupIterPtr uint32

		for cgNo, egRefNo := range groups[egNo] { // client group
			finch.Debug("alloc %d/%d eg ref %d", egNo, cgNo, egRefNo)
			runlevel.ClientGroup = uint(cgNo + 1)
			cg := a.Workload[egRefNo]

			clientsQPS := limit.And(execGroupQPS, limit.NewRate(cg.QPSClients))
			clientsTPS := limit.And(execGroupTPS, limit.NewRate(cg.TPSClients))

			clients[egNo][cgNo].Runtime, _ = time.ParseDuration(cg.Runtime)
			clients[egNo][cgNo].Clients = make([]*client.Client, cg.Clients)

			var clientsIterPtr uint32

			for k := uint(0); k < cg.Clients; k++ { // client
				runlevel.Client = k + 1

				// Make a new sql.DB and sql.Conn for this client. Yes, sql.DB are meant
				// to be shared, but this is a benchmark tool and each client is meant to be
				// isolated. So we duplicate per client so there's zero chance of one client
				// affecting another via a shared sql.DB. The connection to MySQL was already
				// tested in compute/Instance.Boot, so these should not error.
				db, _, err := dbconn.Make()
				if err != nil {
					return nil, err
				}

				c := &client.Client{
					RunLevel: *runlevel, // copy
					DB:       db,
					Iter:     cg.Iter,
					DoneChan: a.DoneChan,
					Stats:    make([]*stats.Trx, len(cg.Trx)), // stats per trx
				}

				// Set combined limits, if any: iterations, QPS, TPS
				if cg.IterClients > 0 {
					c.IterClients = cg.IterClients
					c.IterClientsPtr = &clientsIterPtr
				}
				if cg.IterExecGroup > 0 {
					c.IterExecGroup = cg.IterExecGroup
					c.IterExecGroupPtr = &execGroupIterPtr
				}
				if qps := limit.And(clientsQPS, limit.NewRate(cg.QPS)); qps != nil {
					c.QPS = qps.Allow()
				}
				if tps := limit.And(clientsTPS, limit.NewRate(cg.TPS)); tps != nil {
					c.TPS = tps.Allow()
				}

				// Copy statements from transactions assigned to this client,
				// which can be a subset of all trx (config.stage.trx) and in
				// a different order.
				n := 0
				for _, trxName := range cg.Trx {
					n += len(a.TrxSet.Statements[trxName])
				}
				c.Statements = make([]*trx.Statement, n)
				c.Data = make([]trx.Data, n)

				n = 0
				for trxNo, trxName := range cg.Trx {
					runlevel.Trx += 1
					runlevel.TrxName = trxName
					runlevel.Query = 0
					c.Data[n].TrxBoundary |= finch.TRX_BEGIN // finch trx file, not MySQL trx
					if a.Stage == "benchmark" {
						c.Stats[trxNo] = stats.NewTrx(trxName)
					}
					for _, stmt := range a.TrxSet.Statements[trxName] {
						runlevel.Query += 1
						finch.Debug("--- %s", runlevel)
						c.Statements[n] = stmt // *Statement pointer; don't modify

						if len(stmt.Inputs) > 0 {
							c.Data[n].Inputs = []data.Generator{}
							for _, dataKey := range stmt.Inputs {
								if g := a.TrxSet.Data.Copy(dataKey, *runlevel); g != nil {
									c.Data[n].Inputs = append(c.Data[n].Inputs, g)
									if a.TrxSet.Data.Keys[dataKey].Column >= 0 {
										finch.Debug("    input %s <- %s", g.Id().String(), a.TrxSet.Data.Keys[dataKey])
									} else {
										finch.Debug("    input %s", g.Id().String())
									}
								}
							}
						}

						if len(stmt.Outputs) > 0 {
							// For every output (saved column), there's a data generator
							// that accepts the value from the query (output) then acts
							// as the input to another query.
							c.Data[n].Outputs = make([]interface{}, len(stmt.Outputs))
							for i, dataKey := range stmt.Outputs {
								if dataKey == stmt.InsertId {
									continue
								}
								if g := a.TrxSet.Data.Copy(dataKey, *runlevel); g != nil {
									c.Data[n].Outputs[i] = g
									finch.Debug("    output %s", g.Id().String())
								}
							}
						}

						if stmt.InsertId != "" {
							g := a.TrxSet.Data.Copy(stmt.InsertId, *runlevel)
							c.Data[n].InsertId = g
							finch.Debug("    insert-id %s", g.Id().String())
						}

						n++
					} // query
					c.Data[n-1].TrxBoundary |= finch.TRX_END // finch trx file, not MySQL trx
				} // trx

				clients[egNo][cgNo].Clients[k] = c

			} // client
		} // client group
	} // exec group

	return clients, nil
}
