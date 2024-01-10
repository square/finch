---
weight: 2
---

Data keys have a scope corresponding to [run levels]({{< relref "intro/concepts#run-levels" >}}) plus two special low-level scopes:

```
global
└──stage
   └──workload
      └──exec-group
         └──client-group
            └──client
               └──iter
                  └──trx
                     └──statement (default)
                        └──row
                           └──value
```

<mark><b>A data key is unique within its scope, and its data generator is called once per scope iteration.</b></mark>


|Data Scope|Scope Iteration (Data Generator Called When)|Class|
|----------|--------------------------------------------|-----|
|[global](#global)|Finch runs|[One time](#one-time)|
|[stage](#stage)|Stage runs|[One time](#one-time)|
|[workload](#workload)|Each client starts a new iter|[Multi client](#multi-client)|
|[exec-group](#exec-group)|Each client starts a new iter|[Multi client](#multi-client)|
|[client-group](#client-group)|Each client starts a new iter|[Multi client](#multi-client)|
|[client](#client)|Client connects to MySQL or recoverable query error|[Single client](#single-client)|
|[iter](#iter)|Iter count increases|[Single client](#single-client)|
|[trx](#trx)|Trx count increases|[Single client](#single-client)|
|[statement](#statement)|Statement executes|[Single client](#single-client)|
|[row](#row)|Each @d per row when statement executes|[Special](#special)|
|[value](#value)|Each @d when statement executes|[Special](#special)|

For example, @d with statement scope (the default) is called once per statement execution.
Or, @d with iter scope is called once per client iter (start of executing all trx assigned to the client).

As this rest of this page will show, data scope makes it possible to craft both simple and elaborate workloads.

## Configure

Specify [`stage.trx[].data.d.scope`]({{< relref "syntax/stage-file#dscope" >}}) for a data key:

```yaml
stage:
  trx:
    - file:
      data:
        d:
          generator: int  # Required data generator name
          scope: trx      # Optional data scope
```

If not specified, [statement](#statement) is the default scope.

## Explicit Call

|Implicit Call|Explicit Call|
|-------------|-------------|
|`@d`|`@d()`|

By default, each data key is called once per scope iteration because `@d` is an _implicit call_: Finch calls the data generator when the scope iteration changes.
Implicit calls are the default and canonical case because it just works for the vast majority of benchmarks.

An _explicit call_, like `@d()`, calls the data generator regardless of the scope iteration but always within the data scope.
For example, presume `@d` has statement scope and uses the [`auto-inc` generator]({{< relref "data/generators#auto-inc" >}}):

{{< columns >}}
```sql
SELECT @d @d
-- SELECT 1 1
```
<---> <!-- magic separator, between columns -->
```sql
SELECT @d @d()
-- SELECT 1 2
```

{{< /columns >}}

The left query returns `1 1` because the second `@d` is an implicit call and the scope (statement) has not changed.
The right query returns `1 2` because the second `@d()` is an explicit call, so Finch calls the data generator again even though the scope iteration hasn't changed.

[Row scope](#row) is a good example of why explicit calls are sometimes required.
Until then, the rest of this page will continue to default to the canonical case: implicit calls, `@d`.

## Single Client

To reason about and explain single client data scopes, let's use a <mark>canonical example</mark>:

{{< columns >}}
_Client 1_
```
-- Iter 1

-- Trx A n=1
INSERT @a, @R
UPDATE @a, @R

-- Trx B n=2
DELETE @a, @R
```

```
-- Iter 2

-- Trx A n=3
INSERT @a, @R
UPDATE @a, @R

-- Trx B n=4
DELETE @a, @R
```
<--->
&nbsp;
{{< /columns >}}

The client executes two trx: trx A is an `INSERT` and an `UPDATE`; trx B is a `DELETE`.
There's a trx counter: `n=1`, `n=2`, and so forth.
Remember that the statements shown in iter 1 are the same as in iter 2: it's one `INSERT` executed twice; one `UPDATE` executed twice; one `DELETE` executed twice.

There are two data keys:
* @a uses the [`auto-inc` generator]({{< relref "data/generators#auto-inc" >}}) so we know what values it returns when called: 1, 2, 3, and so on.
* @R uses the [random `int` generator]({{< relref "data/generators#int" >}}) so we don't know what value it returns when called. 

These different data keys will help highlight how data scope works and why it's important.

### Statement

Using the canonical example described above, the default data scope (statement) returns:

{{< columns >}}
_Client 1_
```
-- Iter 1

-- Trx A n=1
INSERT @a=1, @R=101
UPDATE @a=1, @R=492

-- Trx B n=2
DELETE @a=1, @R=239
```

```
-- Iter 2

-- Trx A n=3
INSERT @a=2, @R=934
UPDATE @a=2, @R=111

-- Trx B n=4
DELETE @a=2, @R=202
```
<--->
&nbsp;
{{< /columns >}}

First notice that `@a=1` for all three statements in iter 1.
This is because @a is statement scoped: each @a in each statement is unique, so there are three data generators and they each return the first value: 1.
Another way to look at it: statement scoped @a in `INSERT @a` is only visible and accessible within that statement; none of the other statements can see or access it.
The values (in iter 1) are all 1 only because each statement scoped @a happens to return that as the first value.

@R demonstrates why statement scope is the default.
Typically, benchmarks uses random values and expect different (random) values for each query.
Since @R is statement scope, the typical case is generated by default.
A better example for @R is something like:

```sql
SELECT c FROM t WHERE id = @R
```

That might benchmark random lookups on column `id`.

### Trx

Using the canonical example described above, trx scope returns:

{{< columns >}}
_Client 1_
```
-- Iter 1

-- Trx A n=1
INSERT @a=1, @R=505
UPDATE @a=1, @R=505

-- Trx B n=2
DELETE @a=1, @R=293
```

```
-- Iter 2

-- Trx A n=3
INSERT @a=2, @R=821
UPDATE @a=2, @R=821

-- Trx B n=4
DELETE @a=2, @R=410
```
<--->
&nbsp;
{{< /columns >}}

{{< hint type=note >}}
Remember that "trx" in these docs refers to a [Finch trx]({{< relref "benchmark/trx" >}}), not a MySQL transaction, although the two are closely related.
{{< /hint >}}

First again, notice that `@a=1` in the first trx (A) and second trx (B).
This is because @a is trx scoped, so it's unique to each trx (A and B) and called once per trx (when the trx count `n` increases).
But since @a returns the same initial values, it looks the same, so @R demonstrates trx scope better.

Although @R generates random integers, when it's trx scoped it's called only once per trx, so @R in each trx has the same value, like `@R=505` in client 1 iter 1 trx n=1.
This could be used, for example, to benchmark inserting, updating, and deleting random rows, like:

```sql
INSERT INTO t (id, ...) VALUES (@R, ...)
UPDATE t SET ... WHERE id=@R
DELETE FROM t WHERE id=@R
```

Fun fact: the classic [sysbench write-only benchmark]({{< relref "benchmark/examples#sysbench" >}}) does this: it deletes a random row then re-inserts it.
See `@del_id` in its config.

### Iter

Using the canonical example described above, iter scope returns:

{{< columns >}}
_Client 1_
```
-- Iter 1

-- Trx A n=1
INSERT @a=1, @R=505
UPDATE @a=1, @R=505

-- Trx B n=2
DELETE @a=1, @R=505
```

```
-- Iter 2

-- Trx A n=3
INSERT @a=2, @R=821
UPDATE @a=2, @R=821

-- Trx B n=4
DELETE @a=2, @R=821
```
<--->
&nbsp;
{{< /columns >}}

An _iteration (iter)_ is one execution of all trx assigned to a client.
In this example, @a and @R are iter scoped, so they're called once per iter (per client).
Values are the same across trx and only change with each new iter.

Iter scope is useful to "share" values across multiple trx.
This is equivalent to combining multiple trx into one and using trx scope.

### Client

Client scope has the same scope as [iter](#iter) (one client) but its scope iteration is unique: when the client connects to MySQL or recovers from a query error.
Client scope increments at least once: when the client first connects to MySQL.
Further increments occur when the client reconnects to MySQL _or_ starts a new iter to recover from certain errors (see [Benchmark / Error Handling]({{< relref "benchmark/error-handling" >}})).

Is this scope useful?
Maybe.
For example, perhaps there's a use case for client scoped @d to handle duplicate key errors or deadlocks&mdash;recoverable errors that start a new iter.
A client scoped @d would know it's a recoverable error and not just the next iteration, whereas an iter scoped data key couldn't know this.

{{< hint type=tip title=iter-on-error >}}
It might be helpful to think of client scope as "iter-on-error".
{{< /hint >}}

## Multi Client

To reason about and explain multi client data scopes, let's add a second client to the [canonical example](#single-client):

{{< columns >}}
_Client 1_
```
-- Iter 1

-- Trx A n=1
INSERT @a, @R
UPDATE @a, @R

-- Trx B n=2
DELETE @a, @R
```

```
-- Iter 2

-- Trx A n=3
INSERT @a, @R
UPDATE @a, @R

-- Trx B n=4
DELETE @a, @R
```
<--->
_Client 2_
```
-- Iter 1

-- Trx A n=1
INSERT @a, @R
UPDATE @a, @R

-- Trx B n=2
DELETE @a, @R
```
{{< /columns >}}

### Client Group

Using the canonical example described immediately above, client-group scope returns:

{{< columns >}}
_Client 1_
```
-- Iter 1

-- Trx A n=1
INSERT @a=1, @R=505
UPDATE @a=1, @R=505

-- Trx B n=2
DELETE @a=1, @R=505
```

```
-- Iter 2

-- Trx A n=3
INSERT @a=3, @R=821
UPDATE @a=3, @R=821

-- Trx B n=4
DELETE @a=3, @R=821
```
<--->
_Client 2_
```
-- Iter 1

-- Trx A n=1
INSERT @a=2, @R=743
UPDATE @a=2, @R=743

-- Trx B n=2
DELETE @a=2, @R=743
```
{{< /columns >}}

With client group scope, @a and @R are unique to the client group, which means their data generators are shared by all clients in the group.
And the client group scope iteration is "Each client starts a new iter", which means their data generators are called when each client starts a new iter.

Presume this call order:

1. Client 1 iter 1
2. Client 2 iter 1
3. Client 1 iter 2

Since @a is shared by all clients in the group and called when each client starts a new iter, that call order explains the values.

Client group scoped data keys are useful with (pseudo) stateful data generators like @a where the call order matters.
This scope allows coordination across multiple clients.
For example, it's necessary to insert values 1..N without duplicates using multiple clients.

With random value generators like @R, client group scope is equivalent to iter scoped presuming no [explicit calls](#explicit-call).

### Exec Group

Exec group scope works the same as [client group scope](#client-group) but is unique to all client groups in the exec group.
But be careful: since different client groups can execute different trx, make sure any data keys shared across client groups make sense.

### Workload

Workload scope works the same as [client group scope](#client-group) but is unique to all exec groups, which means all clients in the stage.
But be careful: since different exec groups can execute different trx, make sure any data keys shared across exec and client groups make sense.

## One Time

### Stage

Stage data scope applies to the entire stage but stage scoped data keys are only called once when the stage starts.
This might be useful for static or one-time values reused across different exec or client groups.
Or, stage scoped data keys can be [called explicitly](#explicit-call).

### Global

Global data scope applies to all stages in a single Finch run.
For example, if Finch is run like `finch stage1.yaml stage2.yaml`, global scope applies to both stages.
Global scoped data keys are only called once (when the first query of the first client of the first stage executes).
Or, global scoped data keys can be [called explicitly](#explicit-call).

{{< hint type=warning >}}
Global data scope does _not_ span [compute instances]({{< relref "operate/client-server" >}}).
It's an interesting idea that could work, but is there a use case for sharing a data value across compute instances?
{{< /hint >}}

<br>

## Special

### Row

Row scope is intended for use with [CSV substitution]({{< relref "syntax/trx-file#csv" >}}) to produce multi-row `INSERT` statements:

{{< columns >}}
_Trx File_ &rarr;
```sql
INSERT INTO t VALUES
/*!csv 2 (@a, ...)*/
```
<---> <!-- magic separator, between columns -->
_Automatic Transformation_ &rarr;
```sql
INSERT INTO t VALUES
   (@a(), ...)
  ,(@a(), ...)
```
<--->
_Resulting Values_
```sql
INSERT INTO t VALUES
   (1, ...) -- row 1
  ,(2, ...) -- row 2
```
{{< /columns >}}

With row scope, the [`auto-inc`]({{< relref "data/generators#auto-inc" >}}) data key, @a, is unique to the _statement_ but called for each _row_.

As shown above, when used with [CSV substitution]({{< relref "syntax/trx-file#csv" >}}), Finch automatically transforms @a to an [explicit call](#explicit-call) in each row.
Moreover, it transforms only the first occurrence of every unique data key in the row.
In this example, `/*!csv N (@a, @a, @a)*/` produces `(1, 1, 1)` for the first row: the first @a is an explicit call, and the latter two @a are copies of the first.

To achieve the same results without [CSV substitution]({{< relref "syntax/trx-file#csv" >}}), use statement scope and explicit calls&mdash;manually write the query like the automatic transformation shown above&mdash;or use [value scope](#value).

### Value

Value scope means every @d has its own unique data generator and is called every time the statement is executed.
This sounds like statement scope with [explicit calls](#explicit-call), but there's a difference:

{{< columns >}}
_Value Scope_
```sql
       @d    @d
        │     │	
SELECT @d,   @d
```
<---> <!-- magic separator, between columns -->
_Statement Scope with Explicit Calls_
```sql
        ┌─ @d ─┐	
        │      │	
SELECT @d(),  @d()
```
{{< /columns >}}

With value scope, each @d has its own data generator.
With statement scope and explicit calls, all @d in the statement share/call the same data generator.

Whether or not this makes a difference depends on the data generator.
For (pseudo) stateful generators like [`auto-inc`]({{< relref "data/generators#auto-inc" >}}), it makes a difference: value scope yields 1 and 1; statement scopes with explicit calls yields 1 and 2.
For random value generators, it might not make a difference, especially since @d can have only one configuration.
If, for example, you want two random numbers with the same generator but configured differently, then you must use two different data keys, one for each configuration.
