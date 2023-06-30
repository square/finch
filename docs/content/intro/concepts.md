---
weight: 2
---

The following concepts are important for understanding how Finch works, and many of them are unique to Finch.

{{< hint type=tip >}}
If you're new to database benchmarking read <a target="_new" href="https://hackmysql.com/benchmarking/">Fundamentals of Database Benchmarking</a>.
{{< /hint >}}

{{< toc >}}

## Run Levels

Finch is designed on a hierarchy of run levels:

```
global
└──stage
   └──workload
      └──exec group
         └──client group
            └──client
               └──trx
                  └──statement
```

These are called _run levels_ because they determine if, when, and how various aspects of Finch run.
By understanding the run levels, you'll understand all the core concepts of Finch and how they work.

Let's start at the bottom because it's the most direct and concrete concept: SQL statements.
Moving up the hierarchy, the concepts become increasingly abstract and specific to Finch.

### Statements

A _statement_ is any SQL statement that Finch can execute on MySQL: `SELECT`, `INSERT`, `UPDATE`, `DELETE`, and so forth.

This is usually synonymous with _query_, but statement is more general because, for example, would you call `BEGIN` a query?
Maybe, but what about `CREATE` or `SET`?

Statements are the lowest level&mdash;but most important part&mdash;of running Finch: executing SQL statements on MySQL.
This shouldn't be surprising since Finch is a benchmarking tool, but what might be surprising is that you write Finch benchmarks using real SQL statements; they are not wrapped in a scripting language.
You write one or more SQL statement in a Finch transaction file.

### Transactions

Finch _transactions (trx)_ are plain text files that contain one or more SQL statement to execute.
Suppose that you want to benchmark these statements:

```sql
SELECT c FROM t WHERE id = @d

BEGIN

SELECT id FROM t WHERE user = @d

UPDATE users SET n = n + 1 WHERE id = @d

COMMIT
```

You can put those statements in one file and it will work, but it's better to separate the transactions:

{{< columns >}}
_trx-1.sql_
```sql
SELECT c FROM t WHERE id = @d
```
<---> <!-- magic separator, between columns -->
_trx-2.sql_
```sql
BEGIN

SELECT id FROM t WHERE user = @d

UPDATE users SET n = n + 1 WHERE id = @d

COMMIT
```
{{< /columns >}}


{{< hint type=note >}}
Ignore the `@d` placeholders for now; they are Finch [data keys](#data-generators) described later on this page.
{{< /hint >}}

With separate Finch trx, you can craft more advanced and interesting benchmarks, like:

|Trx|Clients|TPS|
|---|-------|---|
|trx-1.sql|100|(Unlimited)|
|trx-2.sql|20|50|

Finch will run the first trx (the single `SELECT`) on 100 clients as fast as possible, and it will run the second trx (the explicit, multi-statement SQL transaction) on only 20 clients limited to 50 transactions per second (TPS).

{{< hint type=tip >}}
Remember the example in the table above because it's reused in the following sections.
{{< /hint >}}

Finch also collects and reports statistics per-trx.
(The built-in stats reports combine all trx stats, but a [stats plugin]({{< relref "api/stats" >}}) receives per-trx stats.)

You can write a benchmark as one large pseudo-transaction (all SQL statements in one file), but for various reasons that will become clear later, it's better to write each SQL transaction in a separate Finch trx file.

### Client

A _client_ is one MySQL client (connection) that executes all trx assigned to it.
Since a trx contains statements, a client executes all the statements in all the trx assigned to it.

You can configure Finch to run any number of clients.
This is typical for benchmarking tools, but Finch has two unique features:

* Clients can be grouped (described next)
* Clients can be assigned different trx

The second point makes Finch especially powerful and flexible.
For example, as shown in the table above, you can run 100 clients executing certain transactions, and 20 clients executing other transactions.
Having different clients executing different trx is why Finch needs client and execution groups.

### Client and Execution Groups

A _client group_ is a logical group of clients with the same trx and limits.
From the example in the table above, this snippet of a Finch stage file creates two client groups:

```yaml
workload:
  - clients: 100      # Client group 1
    trx: [trx-1.sql]  # 

  - clients: 20       # Client group 2
    trx: [trx-2.sql]  #
    tps-clients: 50   #
```

The first client group will run 100 independent clients, all executing the statements in trx `trx-1.sql`.
The second client group will run 20 independent clients, all executing the statements in trx `trx-2.sql`, and limited to 50 TPS across all 20 clients.
Together, this will create 120 MySQL clients (connections).

An _execution group_ is one or more client groups with the same group name.

Let's pretend you want to execute another client group _after_ client groups 1 and 2 (above) are done.
That requires putting client groups 1 and 2 in a named execution group&mdash;let's call it `FOO`&mdash;and the third client group in another named execution group&mdash;let's call it `BAR`.
A snippet of a Finch stage file is shown below that creates these two execution groups.

```yaml
workload:
                      # FOO ----------
  - clients: 100      # Client group 1
    trx: [trx-1.sql]  # 
    group: FOO        #
  - clients: 20       # Client group 2
    trx: [trx-2.sql]  #
    tps-clients: 50   #
    group: FOO        #
                         # BAR ----------
  - client: 1            # Client group 3
    trx: [trx-3.sql]     #
    group: BAR           #
```

Finch processes the `workload` list top to bottom, so order matters when creating execution groups.
All client groups in an execution group run concurrently.
And execution groups are run top to bottom in workload order.

Visually, the workload processing (Y axis) and time when the execution groups are run (X axis) can be depicted like:

![Finch Execution Group](/finch/img/finch_exec_group.svg)

Both client groups in execution group `FOO` (blue) run first.
When they're done, the client group in execution group `BAR` runs next.

This is advanced workload orchestration.
Sometimes it's necessary (like with DDL statements), but it's optional for most simple benchmarks that just want run everything all at once.

### Workload

A workload is a combination of queries, data, and access patterns.
In Finch, the previous (lower) run levels combine to create a workload that is defined in the aptly named `workload` section of a stage file:

```yaml
stage:
  workload:
    - clients: 100
      trx: [trx-1.sql]
    - clients: 20
      trx: [trx-2.sql]
      tps-clients: 50 # all 20 clients
  trx:
    - file: trx-1.sql
    - file: trx-2.sql
```

That defines the workload for the running example (the table in [Transactions](#transactions)): 100 clients to execute the single `SELECT` in `trx-1.sql` and, concurrently, 20 clients to execute the transaction in `trx-2.sql` limited to 20 TPS (across all 20 clients).

In short, a Finch workload defines which clients run which transitions, plus a few features like QPS and TPS limiters.

### Stages

A stage executes a workload.
As shown immediately above, a Finch stage is configured in a YAML file.
Here's a real and more complete example:

```yaml
stage:
  name: read-only
  runtime: 60s
  mysql:
    db: finch
  workload:
    - clients: 1
  trx:
    - file: trx/read-only.sql
      data:
        id:
          generator: "rand-int"
          scope: client
          params:
            max: $params.rows
            dist: normal
        id_100:
          generator: "int-range"
          scope: client
          params:
            range: 100
            max: $params.rows
```

Every stage has a name: either set by `stage.name` in the file, or defaulting to the base file name.
Every stage needs at least one trx, defined by the `stage.trx` list.
Every stage should have an explicitly defined workload (`stage.workload`), but in some cases Finch can auto-assign the workload.

The rest is various configuration settings for the stage, workload, data generators, and so forth.

Finch runs one or more stages _sequentially_ as specified on the command line:

```bash
finch STAGE_1 [... STAGE_N]
```

This allows you to do a full setup, benchmark, and cleanup in a single run by writing and specifying those three stages.
But you don't have to; it's common to set up only once but run a benchmark repeatedly, like:

```bash
finch setup.yaml
finch benchmark.yaml
finch benchmark.yaml
finch benchmark.yaml
```

### Compute

The global run level represents one running instances of Finch (all stages) on a compute instance.
What's unique about Finch is that one instance (a server) can coordinate running the stages on other instances (clients):

![Finch Distributed Compute](/finch/img/finch_compute.svg)

This is optional, but it's easy to enable and useful for creating a massive load on a MySQL instance with several smaller and cheaper compute instances rather than one huge benchmark server.

To enable, configure a stage with:

```yaml
compute:
  instances: 2  # 1 server + 1 client
```

Then start a Finch server and one client like:

{{< columns >}}
_Server_
```sh
finch benchmark.yaml
```
<---> <!-- magic separator, between columns -->
_Client_
```sh
finch --server 10.0.0.1
```
{{< /columns >}}

The server sends a copy of the stage to the client, so there's nothing to configure on the client.
The client sends its statistics to the server, so you don't have to aggregate them&mdash;Finch does it automatically.

## Data Generators

A _data generator_ is a plugin that generates data for [statements](#statements).
Data generators are referenced by user-defined _data keys_ like @d.
Data keys and generators are two sides of the same coin.

### Name and Configuration

Data keys are used in statements and configured with a specific data generator in the stage file:

{{< columns >}}
_Statement_
```sql
SELECT c FROM t WHERE id = @d
```
<---> <!-- magic separator, between columns -->
_Stage File_
```yaml
stage:
  trx:
    - file: read.sql
      data:
        d: # @d
          generator: "int"
          params:
            max: 25,000
            dist: uniform
```
{{< /columns >}}

{{< hint type=note >}}
`@d` in a statement, but `d` (no `@` prefix) in a stage file because `@` is a reserved character in YAML.
{{< /hint >}}

In the stage file, `stage.trx[].data.d.generator` configures the type of data generator: data key `@d` uses the `int` (random integer) data generator.
And `stage.trx[].data.d.params` provides generator-specific configuration; in this case, configuring the `int` generator to return a uniform random distribution of numbers between 1 and 25,000.
When Finch clients execute the `SELECT` statement above, data key `@d` will be replaced with random values from this `int` data generator.

Finch ships with [built-in data generators]({{< relref "data/generators" >}}), and it's easy [build your own]({{< relref "api/data" >}}) data generator.

### Scope

Data generators are scoped to a [run level](#run-levels).
To see why data scope is important, consider this transaction:

```sql
BEGIN

SELECT c FROM t WHERE id = @d

UPDATE t SET c=c+1 WHERE id = @d

COMMIT
```

The intention is that @d (the value returned by its data generator) is the same for both the `SELECT` and the `UPDATE`.
If `@d` has statement scope, then it's called once per statement, which means `SELECT @d` and `UPDATE @d` will generate two different values.
But if `@d` has trx scope, then it's called once per transaction, which satisfies the intention.

### Input/Output

Most data generators output values, but they can also act as inputs to receive values.
A common example is saving and reusing an insert ID:

```sql
-- save-insert-id: @d
INSERT INTO money_transfer VALUES (...)

INSERT INTO audit_log VALUES (@d, ...)
```

Finch saves the insert ID returned from MySQL into `@d`.
Then, in the second query, `@d` returns that insert ID value.


## Benchmark Directory

By convention, stages related to the same benchmark are put in the same directory, and [trx files](#transactions) are put in a `trx/` subdirectory.
Here's an example from the Finch repo:

```bash
finch/benchmarks/sysbench % tree .
.
├── _all.yaml
├── read-only.yaml
├── setup.yaml
├── trx
│   ├── insert-rows.sql
│   ├── read-only.sql
│   ├── schema.sql
│   ├── secondary-index.sql
│   └── write-only.sql
└── write-only.yaml
```

That is a recreation of the venerable [sysbench](https://github.com/akopytov/sysbench) benchmarks:

* Stage `setup.yaml` creates the sysbench schema and inserts rows 
* Stage `read-only.yaml` is the sysbench read-only benchmark
* Stage `write-only.yam` is the sysbench write-only benchmark

All trx files are kept under `trx/`.
These are referenced in the stage files with relative paths, like `trx/insert-rows.sql`.

`_all.yaml` is a special meta file with parameters for all stages.

These conventions and relatives paths work because Finch changes working directory to the directory of the stage file.

## Parameters

Parameters let you to define key-value pairs once and reference them in separate places&mdash;the DRY principle: don't repeat yourself.
The simplest example is a single stage and trx file:

{{< columns >}}
_Stage_
```yaml
stage:
  params:
    rows: "10,000"
  trx:
    - file: trx.sql
```
<---> <!-- magic separator, between columns -->
_Trx_
```sql
-- rows: $params.rows
INSERT INTO t (...)
```
{{< /columns >}}

`$params.rows` in the trx is replaced by the `stage.params.rows` value (10,000). 

You can also override the value on the command line:

```bash
finch benchmark.yaml --param rows=500000
```

Parameters are most useful for multi-stage benchmarks because the shared parameters can be defined in `_all.yaml` in the same directory as the stages.
When `_all.yaml` exists, Finch automatically applies its parameters to all stages.

For example, in the [Benchmark Directory](#benchmark-directory) above, parameters in `_all.yaml` are automatically applied to all stages (`setup.yaml`, `read-only.yaml`, and `write-only.yaml`).
When the setup stage is run, `params.rows` number of rows are inserted into the table.
When the other stages are run, random rows between 1 and `params.rows` are accessed.
