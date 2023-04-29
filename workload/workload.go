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
	Stage     uint
	StageName string
	TrxSet    *trx.Set             // config.stage.trx
	Workload  []config.ClientGroup // config.stage.workload
	StageQPS  limit.Rate           // config.stage.qps
	StageTPS  limit.Rate           // config.stage.tps
	DoneChan  chan *client.Client  // Stage.doneChan
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
	// Special case: no client groups specified (stage.workload=[])
	if len(a.Workload) == 0 {
		a.Workload = a.AutoAssign()
	}

	// Fill in missing client group names. Names are required because they determine
	// exec groups in the follow code block.
	autoNo := 0
	ddlNo := 0
	prevHasDDL := true
	prev := ""
	for i := range a.Workload {
		if len(a.Workload[i].Trx) == 0 {
			finch.Debug("cg %d: all trx", i)
			a.Workload[i].Trx = a.TrxSet.Order
		}

		hasDDL := a.hasDDL(a.Workload[i].Trx)
		if a.Workload[i].Name != "" {
			if hasDDL && finch.Uint(a.Workload[i].Iter) == 0 {
				a.Workload[i].Iter = "1"
			}
			continue
		}

		// Does any trx have DDL?
		if hasDDL {
			ddlNo += 1
			a.Workload[i].Name = fmt.Sprintf("ddl%d", ddlNo)
			prevHasDDL = true
		} else {
			if prevHasDDL {
				autoNo += 1
				a.Workload[i].Name = fmt.Sprintf("dml%d", autoNo)
				prevHasDDL = false
			} else {
				a.Workload[i].Name = prev
			}
		}
		prev = a.Workload[i].Name
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

	// Debug print because exec/client groups are complicated and important
	for i := range a.Workload {
		finch.Debug("%3d exec group: %+v", i, a.Workload[i])
	}
	finch.Debug("exec groups: %+v", groups)

	return groups, nil
}

func (a *Allocator) Clients(groups [][]int, withStats bool) ([][]ClientGroup, error) {
	finch.Debug("clients %v with stats %t", groups, withStats)

	clients := make([][]ClientGroup, len(groups))
	runlevel := &finch.RunLevel{
		Stage:         a.Stage,
		StageName:     a.StageName,
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

		// Wherever you see finch.Uint, the string value (e.g. "100") has already been
		// validated, so this func is just a shortcut to return uint rather than uint, erroor.
		execGroupQPS := limit.And(a.StageQPS, limit.NewRate(finch.Uint(cgFirst.QPSExecGroup)))
		execGroupTPS := limit.And(a.StageTPS, limit.NewRate(finch.Uint(cgFirst.TPSExecGroup)))

		clients[egNo] = make([]ClientGroup, len(groups[egNo]))

		var execGroupIterPtr uint32

		for cgNo, egRefNo := range groups[egNo] { // client group
			finch.Debug("alloc %d/%d eg ref %d", egNo, cgNo, egRefNo)
			runlevel.ClientGroup = uint(cgNo + 1)
			cg := a.Workload[egRefNo]

			clientsQPS := limit.And(execGroupQPS, limit.NewRate(finch.Uint(cg.QPSClients)))
			clientsTPS := limit.And(execGroupTPS, limit.NewRate(finch.Uint(cg.TPSClients)))

			nClients := finch.Uint(cg.Clients)
			clients[egNo][cgNo].Clients = make([]*client.Client, nClients)
			clients[egNo][cgNo].Runtime, _ = time.ParseDuration(cg.Runtime) // already validated

			var clientsIterPtr uint32

			for k := uint(0); k < nClients; k++ { // client
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
					RunLevel:  *runlevel,  // copy
					DB:        db,         // *sql.DB
					DefaultDb: cg.Db,      // default database
					DoneChan:  a.DoneChan, // <- *Client
					Iter:      finch.Uint(cg.Iter),
					Stats:     make([]*stats.Trx, len(cg.Trx)), // Client requires slice but values can be nil
				}

				// Set combined limits, if any: iterations, QPS, TPS
				if n := finch.Uint(cg.IterClients); n > 0 {
					c.IterClients = uint32(n)
					c.IterClientsPtr = &clientsIterPtr
				}
				if n := finch.Uint(cg.IterExecGroup); n > 0 {
					c.IterExecGroup = uint32(n)
					c.IterExecGroupPtr = &execGroupIterPtr
				}
				if qps := limit.And(clientsQPS, limit.NewRate(finch.Uint(cg.QPS))); qps != nil {
					c.QPS = qps.Allow()
				}
				if tps := limit.And(clientsTPS, limit.NewRate(finch.Uint(cg.TPS))); tps != nil {
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
				c.Data = make([]client.StatementData, n)

				n = 0
				for trxNo, trxName := range cg.Trx {
					runlevel.Trx += 1
					runlevel.TrxName = trxName
					runlevel.Query = 0
					c.Data[n].TrxBoundary |= trx.BEGIN // finch trx file, not MySQL trx
					if withStats {
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
					c.Data[n-1].TrxBoundary |= trx.END // finch trx file, not MySQL trx
				} // trx

				clients[egNo][cgNo].Clients[k] = c

			} // client
		} // client group
	} // exec group

	return clients, nil
}

func (a *Allocator) AutoAssign() []config.ClientGroup {
	cg := []config.ClientGroup{}
	prevHasDDL := true
	for _, trxName := range a.TrxSet.Order {
		if a.TrxSet.Meta[trxName].DDL {
			// Trx with DDL, must exec alone
			finch.Debug("auto: %s (DDL)", trxName)
			cg = append(cg, config.ClientGroup{
				Clients: "1",
				Iter:    "1",
				Trx:     []string{trxName},
			})
			prevHasDDL = true
		} else {
			if prevHasDDL {
				// Switch to non-DDL
				finch.Debug("auto: %s", trxName)
				cg = append(cg, config.ClientGroup{
					Clients: "1",
					Trx:     []string{trxName},
				})
			} else {
				finch.Debug("auto: %s", trxName)
				last := len(cg) - 1
				cg[last].Trx = append(cg[last].Trx, trxName)
			}
			prevHasDDL = false
		}
	}

	/*

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
	*/

	return cg
}

func (a *Allocator) hasDDL(trxNames []string) bool {
	for _, trxName := range trxNames {
		if a.TrxSet.Meta[trxName].DDL {
			return true
		}
	}
	return false
}
