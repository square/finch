---
weight: 1
---

Trx files are plain text format with blank lines separating SQL statements.

{{< hint type=tip >}}
Use a `.sql` file extension to enable SQL syntax highlighting in your editor.
{{< /hint >}}

{{< toc >}}

## Format

* Statements must be separated by one or more blank lines (example 1)
* A single statement can span multiple lines as long as none of them are blank (example 2)
* Statement modifiers are single-line SQL comments immediately preceding the statement (example 3)
* SQL substitutions are one line `/*! ... */` SQL comments embedded in the statement (example 4)

{{< columns >}}
_Example 1_
```sql
SELECT q FROM t WHERE id = @d

DELETE FROM t WHERE id = @d
```
<--->
_Example 2_
```sql
INSERT INTO t
(q, r, s)
VALUES
, (@q, 2, 3)
, (@q, 5, 6)
```
{{< /columns >}}

{{< columns >}}
_Example 3_
```sql
BEGIN 

-- save-result: @q, _
SELECT q, r FROM t WHERE id = @d

DELETE FROM t WHERE q = @q

COMMIT
```
<--->
_Example 4_
```sql
-- prepare
-- rows: ${params.rows}
INSERT INTO t VALUES /*!csv 100 (NULL, @d)*/
```
{{< /columns >}}

## Statement Modifiers

Statement modifiers modify how Finch executes and handles a statement.
They are all optional, but most benchmarks use a few of them, especially during a setup stage.

### copies

`-- copies: N` 

Add N-1 copies to output
{.tagline}

{{< columns >}}
_Input_ &rarr;
```sql
-- copies: 3
SELECT foo
```
<--->
_Output_
```sql
SELECT foo

SELECT foo

SELECT foo
```
{{< /columns >}}

`N` is the _total_ number of statements in the output:

|N|Output|
|-|------|
|&lt; 0|(Error)|
|1|No change; first statement|
|&ge; 2|First statement plus N-1 more|
{.compact .params}

If you want 10 copies of the same statement, you can write the statement (and its SQL modifiers) 10 times.
Or you can write it once, add `-- copies: 10`, and Finch will automatically write the other 9 copies.

The copies are identical as if you had written all N of them.
The only difference is when this modifier is combined with [`prepare`](#prepare): prepare on copies make a single prepared statement for all copies.

### database-size

`-- database-size: DB SIZE`

Insert rows until database reaches a certain size

|Variable|Value|
|--------|-----|
|`DB`|Unquoted database name to check|
|`SIZE`|[string-int]({{< relref "syntax/values#string-int" >}}) &gt; 0 (examples: 100MB, 5.1GB, 2TB)|

After the table size is reached, the client stops even if other [limits]({{< relref "data/limits" >}}) have not been reached.

The size is not exact because it's checked periodically.
The final size is usually a little larger, but not by much.

### idle

`-- idle: TIME`

Sleep for some time
{.tagline}

`TIME` is a [time duration]({{< relref "syntax/values#time-duration" >}}), like "5ms" for 5 millisecond.

This is useful to simulate known delays, stalls, or latencies in application code.
It's also useful to benchmark the effects of migrating to a slower environment, like migrating MySQL from bare metal with local storage to the cloud with network storage.

An idle sleep does _not_ count as a query, and it's not directly measured or reported in [statistics]({{< relref "benchmark/statistics" >}}).

### prepare

`-- prepare`

Use a prepared statement to execute the query
{.tagline}

By default, Finch does not use prepared statements: data keys (@d) are replaced with generated values, and the whole SQL statement string is sent to MySQL.
But with `-- prepare`, data keys become SQL parameters (?), Finch prepares the SQL statement, and uses generated values for the SQL parameters.

### rows

`-- rows: N`

Insert N many rows
{.tagline}

After N rows, the client stops even if other [limits]({{< relref "data/limits" >}}) have not been reached.

### save-columns

`-- save-columns: @d, _`

Save columns into corresponding data keys, or "_" to ignore
{.tagline}

For SELECT statements, the built-in [column data generator]({{< relref "data/generators#column" >}}) can save column values from the _last row_ of the result set.
Every column must have a corresponding data key or "\_" to ignore the column:

```sql
-- save-columns: @d, _, @q
SELECT a, b, c FROM t WHERE ...
```

The example above saves `a` &rarr; @d, ignores `b`, and `c` &rarr; @q&mdash;from the _last row_ of the result set.
(@d and @q must be defined in the stage file and use the column generator, but "\_" is not defined anywhere.)
Since the column data generator defaults to [trx data scope]({{< relref "data/scope#trx" >}}), you can use @d and @q in another statement in the same trx.

{{< hint type=note >}}
The default [data scope]({{< relref "data/scope" >}}) for column data is _trx_, not statement.
{{< /hint >}}

By default, only column values from the last row of the result set are changed, but all rows are scanned.
Therefore, you can implement a [custom data generator]({{< relref "api/data" >}}) to save the entire result set.

### save-insert-id

`-- save-insert-id: @d`

Save insert ID as @d
{.tagline}

This only works for a single row INSERT, and @d must be configured to use the [column data generator]({{< relref "data/generators#column" >}}).

As a silly example, this trx inserts a row then deletes it:

```sql
-- save-insert-id: @d
INSERT INTO t VALUES (...)

DELETE FROM t WHERE id = @d
```

### table-size

`-- table-size: TABLE SIZE`

Insert rows until table reaches a certain size
{.tagline}

|Variable|Value|
|--------|-----|
|`TABLE`|Unquoted table name to check (can be database-qualified)|
|`SIZE`|[string-int]({{< relref "syntax/values#string-int" >}}) &gt; 0 (examples: 100MB, 5.1GB, 2TB)|

After the table size is reached, the client stops even if other [limits]({{< relref "data/limits" >}}) have not been reached.

The size is not exact because it's checked periodically.
The final size is usually a little larger, but not by much.

## SQL Substitutions

SQL substitutions change parts of the SQL statement.

### copy-number

`/*!copy-number*/`

Replaced by the [copy number](#copies).

This can be used anywhere, including a table definition like:

{{< columns >}}
_Input_ &rarr;
```sql
-- copies: 2
CREATE TABLE t/*!copy-number*/ (
  ...
)
```
<--->
_Output_
```sql
CREATE TABLE t1 (
  ...
)

CREATE TABLE t2 (
  ...
)
```
{{< /columns >}}

### csv

`/*!csv N (COLS)*/`

Writes `N` many `(COLS)`, comma-separated, to generate multi-row INSERT statements.

{{< columns >}}
_Input_ &rarr;
```sql
INSERT INTO t VALUES /*!csv 3 (a, @d)*/
```
<--->
_Output_
```sql
INSERT INTO t VALUES (a, @d), (a, @d), (a, @d)
```
{{< /columns >}}


By default, Finch uses [row scope]({{< relref "data/scope#row" >}}) for all data keys in a CSV substitution.
This is usually correct, but you can override by configuring an explicit data scope&mdash;but doing so might produce duplicate values/rows.

Finch does not auto-detect manually written multi-row INSERT statements.
For these, you probably want [statement scope]({{< relref "data/scope#statement" >}}) plus [explicit calls]({{< relref "data/scope#explicit-call" >}}).

{{< hint type=tip >}}
The fastest way to bulk-insert rows is by combining CSV substitution, [prepare](#prepare), and multiple clients.
`N = 1000` or more is possible; the limiting factor is MySQL system variable [`max_allowed_packet`](https://dev.mysql.com/doc/refman/en/server-system-variables.html#sysvar_max_allowed_packet).
{{< /hint >}}
