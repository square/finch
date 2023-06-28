---
weight: 5
---

|Stat|Type|Unit|Measures|
|----|----|----|--------|
|interval|uint|-|Monotonically increasing interval number|
|duration|float64|seconds|Length of interval|
|runtime|float64|seconds|Running total runtime (all intervals)|
|clients|uint|-|Number of clients reporting stats|
|QPS|int64|-|Queries per second, all SQL statements|
|min|int64|microseconds (&micro;s)|Minimum query response time|
|P999|int64|microseconds (&micro;s)|99.9th [percentile](#percentiles) query response time|
|max|int64|microseconds (&micro;s)|Maximum query response time|
|r_QPS|int64|-|Read queries per second (`SELECT`)|
|r_min|int64|microseconds (&micro;s)|Minimum read response time|
|r_P999|int64|microseconds (&micro;s)|99.9th  [percentile](#percentiles) read response time|
|r_max|int64|microseconds (&micro;s)|Maximum read response time|
|w_QPS|int64|-|Read queries per second|
|w_min|int64|microseconds (&micro;s)|Minimum read response time|
|w_P999|int64|microseconds (&micro;s)|99.9th  [percentile](#percentiles) read response time|
|w_max|int64|microseconds (&micro;s)|Maximum read response time|
|TPS|int64|-|Transactions per second, measured on `COMMIT` (c)|
|c_min|int64|microseconds (&micro;s)|Minimum `COMMIT` response time|
|c_P999|int64|microseconds (&micro;s)|99.9th  [percentile](#percentiles) `COMMIT` response time|
|c_max|int64|microseconds (&micro;s)|Maximum `COMMIT` response time|
|errors|uint64|-|Number of errors caused by query execution|
|N|uint64|-|Number of queries executed (not reported)|
|compute|string|-|Compute hostname, or "(# combined)"|

## Percentiles

The default percentile is P999 (99.9th), but [built-in reporters](#reporters) support a variable list of percentiles.
For example, if a built-in reporter is configured for "P95,P99,P999", then it will report 3 percentiles for each breakdown: all queries, read queries (`SELECT`), write queries, and `COMMIT`.

Percentiles are calculated using the same [MySQL 8.0 histogram buckets](https://dev.mysql.com/doc/mysql-perfschema-excerpt/8.0/en/performance-schema-statement-histogram-summary-tables.html): 450 buckets increasing by roughly 4.7%.
For technical details, see [WL#5384: PERFORMANCE_SCHEMA Histograms](https://dev.mysql.com/worklog/task/?id=5384).

Percentile response time values are not exact.
But in the cloud where sub-millisecond response time is rare due to network latency, the values are usually more precise than needed.
For example, the first three 1 millisecond buckets are only ~50&micro;s large:

```
[1000.000000, 1047.128548)  47µs
[1047.128548, 1096.478196)  49µs
[1096.478196, 1148.153621)  51µs
[1148.153621, 1202.264435)  54µs
```

{{< hint type=note >}}
Min and max stats are exact, recorded from actual measurements, not buckets.
{{< /hint >}}

## Aggregation

Finch collects stats per trx&mdash;called _trx stats_.
Trx stats from all clients on a compute instance are aggregated.
That means stats for trx A, for example, reflect the values from all clients executing trx A.
(There are no stats per execution group or client group.)
All stats are recorded even if some don't apply to a trx.
For example, trx C in the diagram below is a single `SELECT` statement, so its write and `COMMIT` stats will be zero.

If the compute instance is a client, it sends its trx stats to the server, and the server aggregates all trx stats from all instances.

![Finch stats combines](/finch/img/finch_stats_combined.svg)

By default, the [built-in reporters](#reports) also aggregate all trx stats for reporting.
As shown in the diagram above, the trx stats for A, B, and C are combined and reported as one set of stats per compute.
(The server is called "local" by default.)
Also, the [stdout report](#stdout) reports both per-compute stats and all compute stats aggregated together: "(2 combined)".

{{< hint type=note >}}
[Percentiles](#percentiles) are aggregated properly by combining bucket counts.
{{< /hint >}}

The combined compute stats are what is typically expected as benchmark stats, but with a [custom reporter]({{< relref "api/stats" >}}) it's possible to report stats per compute, per trx.

## Frequency

By default, Finch reports stats when the stage completes.
(See [Benchmark / Workload / Runtime Limits]({{< relref "benchmark/workload#runtime-limits" >}}).)
This implies 1 interval equal to the entire runtime (duration = runtime in the stats).

Set [`stats.freq`]({{< relref "syntax/all-file#freq" >}}) to enable periodic stats.
For example, if set to "5s", Finch will report stats at 5 second intervals.
Stats are reset each interval; they're not averaged or carried over.
For example, r_max for each interval is the maximum `SELECT` response time for that interval.

{{< hint type=tip >}}
Use periodic stats and the [CSV reporter](#csv) to graph results with an external tool.
{{< /hint >}}

## Reporters

Reports are configured in [`stats.report`]({{< relref "syntax/all-file#report" >}}).
Different reporters can be used at the same time, but only one instance of each reporter.

### stdout

|Param|Default|Valid|
|-----|-------|-----|
|combined|yes|[string-bool]({{< relref "syntax/values#string-bool" >}})|
|each-instance|no|[string-bool]({{< relref "syntax/values#string-bool" >}})|
|percentiles|P999|Comma-spearted Pn values where 1 &ge; n &le; 100|
{.compact .params}

The stdout reporter dumps stats to stdout in a table:

```
 interval| duration| runtime| clients|   QPS| min|  P999|    max| r_QPS| r_min| r_P999|  r_max| w_QPS| w_min| w_P999|  w_max|   TPS| c_min| c_P999|  c_max| errors|compute
        1|     20.0|    20.0|       4| 9,461|  80| 1,659| 79,518| 2,365|   148|  1,096| 37,598| 2,365|   184|  1,202| 40,770| 2,365|   366|  2,398| 79,518|      0|local
```

This is the default reporter and output if no [`stats`]({{< relref "syntax/all-file#stats" >}}) are configured.

### csv

|Param|Default|Valid|
|-----|-------|-----|
|file|finch-benchmark-TIMESTAMP.csv|file name|
|percentiles|P999|Comma-spearted Pn values where 1 &ge; n &le; 100|
{.compact .params}

The csv reporter writes all stats in CSV format to the specified file.
This is used for graphing stats with an external tool when combined with periodic stats: [`stats.freq`]({{< relref "syntax/all-file#freq" >}}) &gt; 0.
Plot runtime on the X axis and other stats on the Y axis (QPS, TPS, and so forth).

The default file is temp file with "TIMESTAMP" replaced by the current timestamp.
If the file exists, Finch exits with an error (to prevent accidentally overwriting stats from previous benchmark runs).

