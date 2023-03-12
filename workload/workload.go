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

type Allocator struct {
	Stage      string
	TrxSet     *trx.Set           // config.trx (this stage)
	ExecGroups []config.ExecGroup // config.stage.workload
	ExecMode   string             // config.stage.exec
	StageQPS   limit.Rate         // config.stage.qps
	StageTPS   limit.Rate         // config.stage.tps
	DoneChan   chan error         // Stage.doneChan
	Auto       bool               // !config.stage.disable-auto-allocation
}

func (a *Allocator) Groups() ([][]int, error) {
	if a.Auto {
		if len(a.ExecGroups) == 0 {
			finch.Debug("auto alloc %s", a.Stage)
			if a.Stage == "setup" || a.Stage == "cleanup" {
				a.ExecGroups = make([]config.ExecGroup, len(a.TrxSet.Order))
				for i, trxName := range a.TrxSet.Order {
					a.ExecGroups[i] = config.ExecGroup{
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
						a.ExecGroups[i].Iter = 0
					}
				}
			} else { // warmup and benchmark
				a.ExecGroups = []config.ExecGroup{
					{
						Clients: 1,
						Trx:     a.TrxSet.Order,
					},
				}
			}
		} else {
			// partial auto alloc (fill in the gaps)
			// Ensure each trx has a client group
			// If one doesn't, give it a default client group
			finch.Debug("partial alloc %s: %+v", a.Stage, a.ExecGroups)

			for i := range a.ExecGroups {
				if len(a.ExecGroups[i].Trx) > 0 {
					continue
				}
				finch.Debug("assign all trx to exec group %d", i)
				a.ExecGroups[i].Trx = make([]string, len(a.TrxSet.Order))
				copy(a.ExecGroups[i].Trx, a.TrxSet.Order)
			}

		TRX:
			for _, trxName := range a.TrxSet.Order {
				for _, execGrp := range a.ExecGroups {
					for i := range execGrp.Trx {
						if execGrp.Trx[i] == trxName {
							continue TRX
						}
					}
				}
				// Not assigned, so assign it
				execGrp := config.ExecGroup{
					Clients: 1,
					Trx:     []string{trxName},
				}
				if a.Stage == "setup" || a.Stage == "cleanup" {
					execGrp.Iter = 1
				}
				a.ExecGroups = append(a.ExecGroups, execGrp)
				finch.Debug("created exec group %d for %s", len(a.ExecGroups)-1, trxName)
			}
		}
	}
	for i := range a.ExecGroups {
		if a.ExecGroups[i].Name == "" {
			a.ExecGroups[i].Name = fmt.Sprintf("e%d", i+1)
		}
		finch.Debug("%3d exec group: %+v", i, a.ExecGroups[i])
	}

	var groups [][]int
	switch a.ExecMode {
	case finch.EXEC_SEQUENTIAL:
		// exec group "name" -> N clients (grouped by exec group name)
		groups = [][]int{}
		groupNo := -1
		groupName := ""
		for i := range a.ExecGroups {
			if a.ExecGroups[i].Name != groupName {
				// New group
				groups = append(groups, []int{})
				groupNo++
				groupName = a.ExecGroups[i].Name
			}
			groups[groupNo] = append(groups[groupNo], i)
		}
		finch.Debug("exec groups: %+v", groups)
	case finch.EXEC_CONCURRENT:
		// 1 exec group -> all clients
		groups = make([][]int, 1)
		groups[0] = make([]int, len(a.ExecGroups))
		for j := range a.ExecGroups {
			groups[0][j] = j
		}
	default:
		// Shouldn't happen because config should have already been validated
		panic("invalid config.exec: " + a.ExecMode)
	}

	return groups, nil
}

func (a *Allocator) Clients(groups [][]int) ([][]*client.Client, error) {
	clients := make([][]*client.Client, len(groups))
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

	var qps, tps limit.Rate

	for i := range groups {
		clients[i] = []*client.Client{}
		runlevel.ExecGroup = uint(i + 1)
		runlevel.ExecGroupName = a.ExecGroups[groups[i][0]].Name

		var execGroupIterPtr uint32

		for cgNo, j := range groups[i] { // client group
			runlevel.ClientGroup = uint(cgNo + 1)

			if j == 0 {
				qps = limit.And(a.StageQPS, limit.NewRate(a.ExecGroups[j].QPSExecGroup))
				tps = limit.And(a.StageTPS, limit.NewRate(a.ExecGroups[j].TPSExecGroup))
			}

			var clientsIterPtr uint32

			qps = limit.And(qps, limit.NewRate(a.ExecGroups[j].QPSClients))
			tps = limit.And(tps, limit.NewRate(a.ExecGroups[j].TPSClients))

			for n := uint(0); n < a.ExecGroups[j].Clients; n++ { // client
				runlevel.Client = n + 1

				// Make a new sql.DB and sql.Conn for this client. Yes, sql.DB are meant
				// to be shared, but this is a benchmark tool and each client is meant to be
				// isolated. So we duplicate per client so there's zero chance of one client
				// affecting another via a shared sql.DB. The connection to MySQL was already
				// tested in compute/Instance.Boot, so these should not error.
				db, _, err := dbconn.Make()
				if err != nil {
					return nil, err
				}

				runtime, _ := time.ParseDuration(a.ExecGroups[j].Runtime) // already validated

				c := &client.Client{
					RunLevel: *runlevel, // copy
					DB:       db,
					Iter:     a.ExecGroups[j].Iter,
					Runtime:  runtime,
					DoneChan: a.DoneChan,
				}
				if a.Stage == "benchmark" {
					c.Stats = stats.NewStats()
				}

				// Set combined limits, if any: iterations, QPS, TPS
				if a.ExecGroups[j].IterClients > 0 {
					c.IterClients = a.ExecGroups[j].IterClients
					c.IterClientsPtr = &clientsIterPtr
				}
				if a.ExecGroups[j].IterExecGroup > 0 {
					c.IterExecGroup = a.ExecGroups[j].IterExecGroup
					c.IterExecGroupPtr = &execGroupIterPtr
				}
				if qps = limit.And(qps, limit.NewRate(a.ExecGroups[j].QPS)); qps != nil {
					c.QPS = qps.Allow()
				}
				if tps = limit.And(tps, limit.NewRate(a.ExecGroups[j].TPS)); tps != nil {
					c.TPS = tps.Allow()
				}

				// Copy statements from transactions assigned to this client,
				// which can be a subset of all trx (config.stage.trx) and in
				// a differnet order
				n := 0
				for _, trxName := range a.ExecGroups[j].Trx {
					n += len(a.TrxSet.Statements[trxName])
				}
				c.Statements = make([]*trx.Statement, n)
				c.Data = make([]trx.Data, n)

				n = 0
				for _, trxName := range a.ExecGroups[j].Trx {
					runlevel.Trx += 1
					runlevel.TrxName = trxName
					runlevel.Query = 0
					c.Data[n].TrxBoundary |= finch.TRX_BEGIN // finch trx file, not MySQL trx
					for _, stmt := range a.TrxSet.Statements[trxName] {
						runlevel.Query += 1
						finch.Debug("--- %s", runlevel)
						c.Statements[n] = stmt // *Statement assigment; don't modify

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

				clients[i] = append(clients[i], c)
			} // client
		} // client group
	} // exec group

	return clients, nil
}
