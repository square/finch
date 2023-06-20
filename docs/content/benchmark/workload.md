---
weight: 4
---

The [`stage.workload`](/syntax/stage-file/#workload) section determines how and when Finch executes transactions.

{{< hint type="tip" >}}
Read  [Client and Execution Groups](/intro/concepts/#client-and-execution-groups) first.
{{< /hint >}}

{{< toc >}}

## Default

The [`stage.workload`](/syntax/stage-file/#workload) section is optional.
If omitted, Finch auto-detects and auto-allocates a default workload:

```yaml
stage:
  trx:         # 
    - file: A  # Explicit
    - file: B  # 

# workload:        # Defaults:
#   - clients: 1   #
#     trx: [A, B]  #  all trx in order
#     name: "..."  #  ddlN or dmlN
#     iter: 0      #  0=unlimited
#     runtime: 0   #  0=forever
```

The default workload runs all trx in _trx order_: the order trx files are specified in [`stage.trx`](/syntax/stage-file/#trx).

The example above is a stage with two trx files: A and B, in that order.
Since there's no explicit workload, the default workload is shown (commented out): 1 client runs both trx (in trx order) forever&mdash;or until you enter CTRL-C to stop the stage.

The default workload is not hard-coded; it's the result of auto-allocation.

## Auto-allocation

If these values are omitted from a client group, Finch automatically allocates:

`clients = 1`
: A client groups needs at least 1 client.

`iter = 1` 
: Only if any assigned trx contains DDL.

`name = ...`
: If any trx contains DDL, the name will be "ddlN" where N is an integer: "ddl1", "ddl2", and so on.
Else, the name will be "dmlN" where N is an integer but only increments in subsequent client groups when broken by a client group with DDL.
For example, if two client groups in a row have only DML, both will be named "dml1" so they form a single execution group.

`trx = stage.trx`
: If a client group is not explicitly assigned any trx, it is auto-assigned all trx in trx order.

## Principles

For brevity, "EG" is execution group and "CG" is client group.

1. A stage must have at least one EG<a id="P1"></a>
1. An EG must have at least one CG<a id="P2"></a>
1. A CG must have a least one client and one assigned trx<a id="P3"></a>
1. CG are read in `stage.workload` order (top to bottom)<a id="P4"></a>
1. A CG must have a name, either auto-assigned or explicitly named<a id="P5"></a>
1. An EG is created by contiguous CG with the same name<a id="P6"></a>
1. EG execute in the order they are created (`stage.workload` order given principles 4&ndash;6)<a id="P7"></a>
1. Only one EG executes at a time<a id="P8"></a>
1. All CG in the same EG execute at the same time (in parallel)<a id="P9"></a>
1. Clients in a CG execute only assigned trx in `workload.[CG].trx` order<a id="P10"></a>
1. An EG finishes when all its CG finish<a id="P11"></a>

These principles are written as P#, like "P1" to refer to "1. A stage must have at least one EG".

## Trx

You can assign any trx to a client group.
Let's say the stage file specifies:

```yaml
stage:
  trx:
    - file: A
    - file: B
    - file: C
```

Given the trx above, the following workloads are valid:

_&check; Any trx order_
```yaml
workload:
  - trx: [B, C, A]
```

_&check; Repeating trx_
```yaml
workload:
  - trx: [A, A, B, B, C]
```

_&check; Reusing trx_
```yaml
workload:
  - trx: [A, B, C]

  - trx: [A, B]
```

_&check; Unassigned trx_
```yaml
workload:
  - trx: [C]
```

## Runtime Limits

Finch runs forever by default, but you probably need results sooner than that.
There are three methods to limit how long Finch runs: runtime (wall clock time), iterations, and data.
Multiple runtime limits are checked with logical _OR_: Finch stops as soon as one limit is reached.

### Runtime

Setting [stage.runtime](/syntax/stage-file/#runtime) will stop the entire stage, even if some execution groups haven't run yet.
Setting [stage.workload.[CG].runtime](/syntax/stage-file/#runtime-1) will stop the client group.
Since execution groups are formed by client groups ([P6](#P6)), this is effectively an execution group runtime limit.
There is no runtime limit for individual clients; if needed, use a client group with `clients: 1`.

### Iterations

One iteration is equal to executing all assigned trx, per client.
If a client is assigned trx A, B, and C, it completes one iteration after executing those three trx.
But if another client is assigned only trx C, then it completes one iteration after executing that one trx.

```yaml
stage:
  workload:
    - iter: N
      iter-clients: N
      iter-exec-group: N
```

`iter` limits each client to N iterations.
`iter-clients` limit all clients in the client group to N iterations.
`iter-exec-group` limits all clients in the execution to N iterations.
Combinations of these three are valid.

{{< hint type="note" >}}
Finch [auto-allocates](#auto-allocation) `iter = 1` for client groups with DDL in an assigned trx.
{{< /hint >}}

### Data

[Data limits](/data/limits) will stop Finch even without a runtime or iterations limit.
When using a data limit, you probably want `runtime = 0` (forever) and `iter = 0` (unlimited) to ensure Finch stops only when the total data size is reached.
And since Finch [auto-allcoates](#auto-allocation) `iter = 1` for client groups with DDL in an assigned trx, you shouldn't mix DDL and DML with a data limit in the same trx because `iter = 1` will stop Finch before the data limit.

## Examples

To focus on the workload, let's presume a stage with three [trx files](../trx/):

```yaml
stage:
  trx:
    - file: A
    - file: B
    - file: C
```

This `stage.trx` section will be presumed and omitted in the following stage file snippets.

### Classic

The classic benchmark workload executes everything all at once:

```yaml
workload:
  - trx: [A, B, C]
```

That executes all three trx at the same time, with one client because [`workload.clients`](/syntax/stage-file/#clients) defaults to 1.
You usually specify more clients:

```yaml
workload:
  - clients: 16
    trx: [A, B, C]
```

That executes 16 clients, all executing all three trx at the same time.

In both cases (1 or 16 clients), the workload is one implicit execution group and one client group.

### Sequential

A sequential workload executes trx one by one.
Given [P7](#P7) and [P8](#P8) and [auto-allocation](#auto-allocation) of DML-only client groups, three different named (explicit) execution groups are needed:

```yaml
workload:
  - trx: [A]
    group: first

  - trx: [B]
    group: second

  - trx: [C]
    group: third
```

Finch executes EG "first", then EG "second", then EG "third".
This type of workload is typical for a [DDL stage](../overview/#ddl) because order is important, but in this case there's an easier way: auto-DDL.

### Auto-DDL

For this example, let's presume:

* A contains a `CREATE TABLE` statement
* B contains a `INSERT` statement (to load rows into the table)
* C contains an `ALTER TABLE` statement (to add a secondary index)

Do _not_ write a `workload` section and, instead, let Finch will automatically generate this workload:

```yaml
workload:
  - trx: [A]    # CREATE TABLE
    group: ddl1 # automatic

  - trx: [B]    # INSERT
    group: dml1 # automatic

  - trx: [C]    # ALTER TABLE
    group: ddl2 # automatic
```

This works because of [auto-allocation](#auto-allocation) and most of the [principles](#principles).

### Parallel Load

Auto-DDL is sufficient when there's not a lot of data to load (or you're very patient).
But if you want to load a lot of data, you need a parallel load workload like:

```yaml
workload:
  - trx: [A]    # Create 2 tables
    group: create

  - trx: [B]    # INSERT INTO table1
    group: rows
    clients: 8
  - trx: [C]    # INSERT INTO table2
    group: rows
    clients: 8
```

Suppose trx A creates two tables.
The first client group is also its own execution group because of `group: create`, and it runs once to create the tables.

The second and third client groups are the same execution group because of `group: rows`, and they execute at the same time ([P9](#P9)).
If trx B inserts into the first table, and trx C inserts into the second table, then 16 clients total will parallel load data.
