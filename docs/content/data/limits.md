---
weight: 4
---

_Data limits_ limit how much or how fast Finch reads or writes data.
By default, Finch has no limits.
But most benchmarks use at least two limits: row count and [`runtime`]({{< relref "syntax/stage-file#runtime" >}}).
Finch has these limits and others.

Multiple limits can be specified.
Execution is stopped (or delayed for throughput limits) when any limit is reached.

{{< toc >}}

## Count

The [`-- rows`]({{< relref "syntax/trx-file#rows" >}}) statement modifier will stop the client after inserting the configured number of rows.
Combine with multiple clients and [CSV expansion]({{< relref "syntax/trx-file#csv" >}}) to bulk insert rows.

{{< hint type=warning >}}
Multi-client, multi-row inserts will exceed the configured row count by one set of multi-rows because the count tracking is async.
If you need the exact number of `-- rows`, use a single client, or submit a PR to improve this feature.
{{< /hint >}}

You can indirectly limit data access with limited iterations:

* [`stage.workload[].iter`]({{< relref "syntax/stage-file#iter" >}})
* [`stage.workload[].iter-clients`]({{< relref "syntax/stage-file#iter-clients" >}})
* [`stage.workload[].iter-exec-group`]({{< relref "syntax/stage-file#iter-exec-group" >}})

For example, `iter = 100` for a single-row `UPDATE` will limit the client to 100 updates.
Or to update a lot of rows quickly, use multiple clients and `iter-clients` to apply a shared limit to all clients.

## Size

Row counts are common but arbitrary.
A thousand rows of a huge table with many secondary indexes and blob columns is significantly different than one million rows of table with a few integer columns.
How much RAM a system has (and MySQL is configured to use) is another factor: even 10 million rows might fit in RAM.

Depending on the benchmark, it might be better to generate certain data sizes, rather than row counts:

* [`database-size`]({{< relref "syntax/trx-file#database-size" >}})
* [`table-size`]({{< relref "syntax/trx-file#table-size" >}})

These statement modifiers are usually used in [DDL stages]({{< relref "benchmark/overview#ddl" >}}).
Combine with a [parallel load]({{< relref "benchmark/workload#parallel-load" >}}) and you can load terabytes of data relatively quickly.
(For benchmarking, "relatively quickly" means hours and days for terabytes of data.)

There are currently no size-based data limits built into any [data generators]({{< relref "data/generators" >}}), but it would be possible to implement for both reading and writing data.

## Throughput

Finch has QPS (queries per second) and TPS (transactions per second) throughput limits.

Use [`stage.qps`]({{< relref "syntax/stage-file#qps" >}}) to ensure that Finch does not exceed a certain QPS no matter the workload or other limits.
This is a top-level limit, and since limits are combined with logical OR, even a higher limit specified in the workload will be limited to `stage.qps`.

The workload-specific QPS limits are:

* [`stage.workload[].qps`]({{< relref "syntax/stage-file#qps-1" >}})
* [`stage.workload[].qps-clients`]({{< relref "syntax/stage-file#qps-clients" >}})
* [`stage.workload[].qps-exec-group`]({{< relref "syntax/stage-file#qps-exec-group" >}})

Finch checks every QPS limit before executing each query.
Delay for QPS throttling is _not_ measured or reported.

The TPS limits are the same, just "tps" instead of "qps":

* [`stage.tps`]({{< relref "syntax/stage-file#tps" >}})
* [`stage.workload[].tps`]({{< relref "syntax/stage-file#tps-1" >}})
* [`stage.workload[].tps-clients`]({{< relref "syntax/stage-file#tps-clients" >}})
* [`stage.workload[].tps-exec-group`]({{< relref "syntax/stage-file#tps-exec-group" >}})

Finch checks all TPS limits on explicit `BEGIN` statements.
And the TPS [statistic]({{< relref "benchmark/statistics" >}}) is measured on explicit `COMMIT` statements.

## Automatic

Finch automatically sets `iter = 1` for a client group with any DDL in any assigned trx.
See [Benchmark / Workload / Auto-allocation]({{< relref "benchmark/workload#auto-allocation" >}}).
