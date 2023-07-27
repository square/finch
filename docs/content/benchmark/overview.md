---
weight: 1
---

A Finch benchmark is defined, configured, and run by one or more stage.


<mark>Finch does not have any built-in, hard-coded, or required stages.</mark>
You write (and name) the stages you need for a benchmark.

{{< hint type=note >}}
This page presumes and requires familiarity with Finch [concepts]({{< relref "intro/concepts" >}}).
{{< /hint >}}

{{< toc >}}

## Two Stages

Benchmarks usually need two stages: one to set up the schema and insert rows, and another to execute queries.
By convention, Finch calls these two stages "**DDL**" and "**standard**", respectively.

The difference is important because it affects how Finch runs a stage _by default_:

|Stage|Exec Order|Runtime Limit|Stats|
|----|-----|-------|-----|
|DDL|Sequential|Data/Rows|No|
|Standard|Concurrent|Time|Yes|

These are only defaults that can be overridden with explicit configuration, but they're correct for most benchmarks.
To see why, let's consider a simple but typical benchmark:

1. Create a table
2. Insert rows
3. Execute queries
4. Report query stats

### DDL

[Data definition language (DDL)](https://dev.mysql.com/doc/refman/en/sql-data-definition-statements.html) refers primarily to `CREATE` and `ALTER` statements that define schemas, tables, and index.
In Finch, a DDL stage applies more broadly to include, for example, `INSERT` statements used only to populate new tables.
(`INSERT` is actually [data manipulation language [DML]](https://dev.mysql.com/doc/refman/en/sql-data-manipulation-statements.html), not DDL.)

A DDL stage executes the first two steps of a typical benchmark:

1. Create a table
2. Insert rows

It's important that these two steps are executed in order and only once per run.
For example, `INSERT INTO t` is invalid _before_ `CREATE TABLE t`.
Likewise, running `CREATE TABLE t` more than once is invalid (and will cause an error).

There's also a finite number of rows to insert.
Benchmark setups don't insert rows for 5 minutes, or some such arbitrary limit, because good benchmarks need more precise data sizes.
Like most benchmark tools, Finch can limit this `INSERT` by row count&mdash;insert 1,000,000 rows for example.
Finch can also insert rows until the table reaches a certain size.
Either way, the `INSERT` runs until a certain data size (or row count) is reached, then it stops.

In Finch, you can achieve these two steps in a one stage and two trx file, like:

{{< columns >}}
_Stage File: setup.yaml_
```yaml
stage:
  trx:
    - file: schema.sql
    - file: rows.sql
```
<--->
_Trx File: schema.sql_
```sql
CREATE TABLE t (
  /* columns */
)
```

_Trx File: rows.sql_
```sql
-- rows: 1,000,000
INSERT INTO t VALUES /* ... * /
```
{{< /columns >}}

The stage lists the trx _in order_: schema first, then rows.
Trx order is important in this case for two reasons.

First, this minimally-configured stage relies on Finch auto-detecting the DDL and executing the workload sequentially in `stage.trx` order.
(This auto-detection can be disabled or overridden, but it's usually correct.)

Second, the `CREATE TABLE` must execute only once, but the `INSERT` needs to execute one millions times.
(Terribly inefficient, but it's just an example.)
The `-- rows: 1,000,000` is a [data limit]({{< relref "data/limits" >}}), and it applies to the Finch trx where it appears, not to the statement.
Intuitively, yes, it should apply only to the statement, but for internal code efficiency reasons it doesn't yet work this way; it applies to the whole trx.
As such, the `INSERT` must be specified in a separate Fix trx file.
But presuming the `INSERT` statement can be executed in parallel, a separate trx file makes it possible to insert in parallel with multiple clients by configuring the workload:

```yaml
stage:
  workload:
    - trx: [schema.sql]
    - trx: [rows.sql]
      clients: 16      # paraellel INSERT
  trx:
    - file: schema.sql
    - file: rows.sql
```

By adding a `stage.workload` section, you tell Finch how to run each trx.
In this case, Finch will execute `schema.sql` once with one client; then it will execute `rows.sql` with 16 clients until 1,000,000 rows have been inserted.

This simple example works, but [Benchmark / Examples]({{< relref "benchmark/examples" >}}) shows better methods and explains some magic happening behind the scenes.

### Standard

A standard stage executes queries, measures response time, and reports the statistics.
This is what engineers think of and expect from a standard database benchmark:

3. Execute queries
4. Report query stats

Finch handles the [stats]({{< relref "benchmark/statistics" >}}), so your focus is queries and how to execute them (the workload).

#### Queries

Finch benchmarks are declarative: write the real SQL statement that you want to benchmark.
Let's imagine that you want to benchmark the two most important read queries from your application.
Put them in a file called `reads.sql` (or whatever name you want):

```
SELECT c1 FROM t WHERE id = @id

SELECT c2 FROM t WHERE n BETWEEN @n AND @PREV LIMIT 10
```

These are fake queries, so don't worry about the details.
The point is: they're real SQL statements; no special benchmark scripting language.

Since you probably already know SQL, you can spend time learning:

1. How and why to model transactions in Finch: [Benchmark / Trx]({{< relref "benchmark/trx" >}})
1. The very simple Finch trx file syntax: [Syntax / Trx File]({{< relref "syntax/trx-file" >}})
1. How to use data generators (`@d`): [Data / Generators]({{< relref "data/generators" >}})

The first two are trivial&mdash;learn once and done.
Data generators can be simple or complex depending on your benchmark.

#### Workload

Queries are inert until executed, and that's what the workload does: declare how to execute the queries (SQL statements written in Finch trx files).
Like data generators, the workload can be simple or complex depending on your benchmark.
Consequently, there's a whole page just for workload: [Benchmark / Workload]({{< relref "benchmark/workload" >}}).

## Other Stages

Two examples of other stages are "warm up" and "tear down".
A warm up stage is typically executed before a standard stage to populate database caches.
A clean up stage is typically executed after a standard stage to remove the schemas created by the DDL stage.

Finch does not have any built-in, hard-coded, or required stages.
You can name your stages (almost) anything and execute them in any order.
Aside from some auto-detection (that can be overridden), Finch treats all stages equally.

## Multi-stage

You can run multiple stages in a single run of Finch:

```bash
finch setup.yaml benchmark.yaml cleanup.yaml
```

That runs stage `setup.yaml`, then stage `benchmark.yaml`, then stage `cleanup.yaml`.
