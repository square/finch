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

### copies

`-- copies: N` 

### database-size

`-- database-size: DB SIZE`

DB is the unquoted database name to check.

SIZE is the maximum size in bytes, supporting human-readable sizes like "500MB" and "1TB".

### idle

`-- idle: TIME`

The idle mod 

### prepare

`-- prepare`

### rows

`-- rows: N`

### save-insert-id

`-- save-insert-id: @d`

Save insert ID as data generator `@d`.

### save-result

`-- save-result: COL_1, ..., COL_N`


### table-size

`-- table-size: TABLE SIZE`

## SQL Substitutions


### copy-number

`/*!copy-number*/`

Replaces

### csv

`/*!csv N (COLS)*/`


