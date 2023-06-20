---
weight: 2
---

Data keys are scoped to the Finch [run levels](/intro/concepts/#run-levels):

```
global
└──stage
   └──workload
      └──exec-group
         └──client-group
            └──client (iter)
               └──trx
                  └──statment
```

Data scope answers two related question:

1. Where is @d unique?
2. When does @d return a new value?

Answer: <mark><b>a data key is unique within its scope and returns a new value when that scope runs.</b></mark>

To help explain (and for brevity), let's use the same notation that Finch uses internally to identify data keys:

`@d(N)/scope`

"N" is the copy number of the generator, and "scope" is the data scope name.
So "@d(1)/trx" is @d, first copy of the generator, trx-scoped.

{{< hint type=tip title="Debug Output" >}}
Normal Finch output doesn't print data IDs, but you can see them with [`--debug`](/command-line/usage/#--debug).
{{< /hint >}}

{{< toc >}}

## Statement

Suppose you have the same trx (with two pseudo-SQL statements for brevity) being executed by 2 clients in the same [client/execution group](/intro/concepts/#client-and-execution-groups):

{{< columns >}}
_Client 1_
```
SELECT @d

UPDATE @d
```
<--->
_Client 2_
```
SELECT @d

UPDATE @d
```
{{< /columns >}}

Data scope determines if @d is the same generator in all four references or different.

|Data Scope|Client 1|Client 2|
|----------|--------|--------|
|statement|@d(1)/statement<br>@d(2)/statement|@d(3)/statement<br>@d(4)/statement|
|trx|@d(1)/trx<br>@d(1)/trx|@d(2)/trx<br>@d(2)/trx|
|client|@d(1)/client<br>@d(1)/client|@d(2)/client<br>@d(2)/client|

Ignore client scope for the moment; we'll come back to it.

1. Where is @d unique?

With statement scope, @d is unique in each statement, evidenced by the copy numbers: "(1)", "(2)", "(3)", and "(4)".
Those mean Finch instantiated four different copies of the data generator.
All four have the same configuration (see [Data / Keys / Duplicates](../keys/#duplicates)), but they are unique and separate instances of the generator, like:

```go
d1 := NewGenerator(config)
d2 := NewGenerator(config)
d3 := NewGenerator(config)
d4 := NewGenerator(config)
```

2. When does @d return a new value?

Since a data generator returns a new value when that scope runs, and the four generators are statement-scoped, then each generator returns a new value when the client runs (executes) its corresponding statement.

When client 1 runs its `SELECT`, @d(1)/statement returns a new value.
When client 1 runs its `SELECT` again, @d(1)/statement returns a new value, and so on for however many times client 1 runs its `SELECT`.
Likewise for its `UPDATE` and for client 2 running its SQL statements.

## Trx

Continuing the previous example, trx scope creates two generators instead of four because it makes @d unique in each trx.
In client 1, the data ID is the same because it's the same data generator: @d(1)/trx for both SQL statements.
Likewise for client 2: @d(2)/trx for both SQL statements.

When client 1 runs its trx, @d(1)/trx returns a new value that is first used for its `SELECT`.
When client 1 runs its `UPDATE`, which is in the same trx, @d(1)/trx returns the same value.
This means the `SELECT` and the `UPDATE` get the same value for @id.

When client 1 runs its trx again, then @d(1)/trx returns a new value and the cycle repeats.
The same happens for client 2.

Trx scope is aptly named because it's how you can "share" (or "sync") generated data in the same trx.

{{< hint type=note >}}
Remember that "trx" in these docs refers to a [Finch trx](/benchmark/trx/), not a MySQL transaction, although the two are closely related.
{{< /hint >}}

What happens if @d appears in more than one trx for the _same client_?

{{< columns >}}
_Client 1, Trx A_
```
SELECT @d

UPDATE @d
```
<--->
_Client 1, Trx B_
```
DELETE @d
```
{{< /columns >}}

Trx-scoped means @d is unique within a trx, even if the client is the same:

|Data Scope|Trx A|Trx B|
|----------|--------|--------|
|trx|@d(1)/trx<br>@d(1)/trx|@d(2)/trx<br>&nbsp;|

This example is important because it leads us to client-scoped data.

## Client

In the first table (in [Statement](#statement)), trx and client data scope seem to have the same data IDs for each client.
The example immediately above&mdash;client 1 with trx A and B&mdash;explains why: client scope makes @d unique in _all trx_ on the client.

|Data Scope|Trx A|Trx B|
|----------|--------|--------|
|client|@d(1)/trx<br>@d(1)/trx|@d(1)/trx<br>&nbsp;|

With client scope, @d in trx B is the same as @d in trx A, as evidenced by the same data ID: @d(1)/trx.

That answers "1. Where is @d unique?", and the answer to "2. When does @d return a new value?" is: on each [iteration](/benchmark/workload/#iterations).
For data scope, "client" and "iter" are synonymous because the main loop of a client iterates through all its [assigned trx](/benchmark/workload/#trx).

## Higher Scopes

```
global
└──stage
   └──workload
      └──exec-group
         └──client-group
```

Data scopes for the higher scopes shown above are not fully implemented because there's been no need for them yet.
It's also unclear how they might work.
For example, client group scope is clear in terms of where @d is unique: all clients, all trx, all statements.
But when should @d return a new value?
Probably after every client in the group has executed one iteration.
But client execution time varies, so this scope would be blocking.

If this changes, this section will be updated.
For now, statement, trx, and client (iter) scope are sufficient to orchestrate complex, realistic workloads.

## Configure

Specify [`stage.trx.[].data.d.scope`](/syntax/stage-file/#dscope) for each data key:

```yaml
stage:
  trx:
    - file:
      data:
        d:
          generator: int
          scope: trx
```
