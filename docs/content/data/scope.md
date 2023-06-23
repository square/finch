---
weight: 2
---

Data keys have four scopes:

```
iter
└──trx
   └──statement (default)
      └──value
```

Data scope answers two related question:

1. Where is @d unique?
2. When is @d called?

Answer: <mark><b>a data key is unique within its scope, and (its generator) is called once per scope execution.</b></mark>

To explain (and for brevity), let's use the same data ID notation that Finch uses internally to identify data keys:

`@d(N)/scope`

"N" is the copy number of the generator, and "scope" is the data scope name.
So "@d(1)/trx" is @d, first copy of the generator, trx-scoped.

{{< hint type=tip title="Debug Output" >}}
Normal Finch output doesn't print data IDs, but you can see them with [`--debug`]({{< relref "command-line/usage#--debug" >}}).
{{< /hint >}}

{{< toc >}}

## Configure

Specify [`stage.trx[].data.d.scope`]({{< relref "syntax/stage-file#dscope" >}}) for each data key:

```yaml
stage:
  trx:
    - file:
      data:
        d:
          generator: int
          scope: trx
```

## Call vs. Value

When the data generator for @d is called, the presumption is that it (the generator) returns a new value.
The [built-in generators]({{< relref "data/generators" >}}) return random and sequential values.
But since data generators are plugins, Finch doesn't know or care if the "new" value is actually new.
For example, a [custom data generator]({{< relref "api/data" >}}) might return values 1, 1, 1, 2, 2, 2, 3, 3, 3&mdash;and so on.
That's valid, but the values aren't always "new".
Consequently, data scope focuses on when (the data generator) for @d is _called_, not if or when it returns a new value.

## Statement

Although value scope is the lowest level scope, statement scope is the default, so let's start here.
Plus, value scope is a special case, so it's explained last.

Suppose you have the same trx (with two pseudo-SQL statements for brevity) being executed by 2 clients in the same [client/execution group]({{< relref "intro/concepts#client-and-execution-groups" >}}):

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

|Data Scope|Statement|Client 1|Client 2|
|----------|---------|--------|--------|
|statement|SELECT<br>UPDATE|@d(1)/statement<br>@d(2)/statement|@d(3)/statement<br>@d(4)/statement|
|trx|SELECT<br>UPDATE|@d(1)/trx<br>@d(1)/trx|@d(2)/trx<br>@d(2)/trx|

Where is @d unique?
With statement scope, @d is unique in each statement, evidenced by the copy numbers: "(1)", "(2)", "(3)", and "(4)".
Those mean Finch instantiated four different copies of the data generator.
All four have the same configuration (see [Data / Keys / Duplicates]({{< relref "data/keys#duplicates" >}})), but they are unique and separate instances of the generator, like:

```go
d1 := NewGenerator(config)
d2 := NewGenerator(config)
d3 := NewGenerator(config)
d4 := NewGenerator(config)
```

When is @d called?
Since a data generator is called once per scope execution, and @d is statement-scoped, then @d is called once per statement&mdash;when the client executes the statement.
When client 1 runs its `SELECT`, Finch calls @d(1)/statement.
When client 1 runs its `SELECT` again, Finch calls @d(1)/statement again, and so on for however many times client 1 runs its `SELECT`.
Likewise for its `UPDATE` and for client 2 running its SQL statements.

And what about the following query?

```sql
SELECT ... WHERE a = @d OR b = @d
```

Since @d is statement-scoped, it's called once per statement, so @d has the same value for both conditions (on both sides of `OR`).

## Trx

Continuing the previous example, trx scope creates two generators instead of four because it makes @d unique in each trx.
In client 1, the data ID is the same because it's the same data generator: @d(1)/trx for both SQL statements.
Likewise for client 2: @d(2)/trx for both SQL statements.

When client 1 runs its trx, Finch calls @d(1)/trx once for the trx, and whatever value @d(1)/trx returns is used in both statements.
When client 1 runs its trx again, Finch @d(1)/trx  again, and the cycle repeats.
The same happens for client 2.

Trx scope is aptly named because it's how you can "share" (or "sync") generated data in the same trx.

{{< hint type=note >}}
Remember that "trx" in these docs refers to a [Finch trx]({{< relref "benchmark/trx" >}}), not a MySQL transaction, although the two are closely related.
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
|trx|SELECT @d(1)/trx<br>UPDATE @d(1)/trx|DELETE @d(2)/trx<br>&nbsp;|

This example is important because it leads us to iter-scoped data.

## Iter

An _iteration (iter)_ is one execution of all trx assigned to a client.
Therefore, iter scope makes @d unique in _all_ assigned trx, per client, and it's called once per iteration.

Compare the example immediately above&mdash;client 1 with trx A and B&mdash;with iter scope instead (the difference is highlighted):

|Data Scope|Trx A|Trx B|
|----------|--------|--------|
|iter|SELECT @d(1)/trx<br>UPDATE @d(1)/trx|DELETE <mark>@d(1)/trx</mark><br>&nbsp;|

With iter scope, @d in trx B is the same as @d in trx A, as evidenced by the same data ID: @d(1)/trx.

## Value

Value scope is a special case for multi-row `INSERT`:

```sql
INSERT INTO t VALUES (@d), (@d), (@d)
```

With statement scope (or higher), every @d would have the same value, but that's probably wrong because the presumptive norm is different row, not duplicate rows.
To achieve different rows, you would need different data keys:

```sql
INSERT INTO t VALUES (@d1), (@d2), (@d3)
```

That works in some cases but not all.
For example, suppose you want to use the [auto-inc generator]({{< relref "data/generators#auto-inc" >}}) to produce rows/values "(1), (2), (3)".
With @d1, @d2, and @d3 being separate data keys and, therefore, separate data generators, it won't work; it will produce "(1), (1), (1)".
You need a single day key, @d, to produce the monotonic sequence. 

Value data scope solves the problem: with value scope, @d is unique per statement and called _per occurrence of @d._

The problem also affects the [CSV substitution]({{< relref "syntax/trx-file#csv" >}}) because it repeats the same data key for each row/value.
As a result, Finch uses value scope automatically for the [CSV substitution]({{< relref "syntax/trx-file#csv" >}}) if you do not explicitly define a scope.

You can explicitly define value scope for any data key, but it might be confusing.
Consider this statement:

```sql
SELECT ... WHERE a = @d OR b = @d
```

With value scope, @d is called twice and likely returns two different values (see [Call vs. Value](#call-vs-value)), but that confuses the relationship between columns `a` and `b` and @d as discussed in [Data / Keys / Name and Configure]({{< relref "data/keys#name-and-configure" >}}):

> a good practice is to name data keys after the columns for which they’re used

So it would be more clear to have `a = @a OR b = @b`, both data keys with the default statement scope.
