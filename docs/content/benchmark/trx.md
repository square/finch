---
weight: 4
---

MySQL transactions and Finch transactions (trx) are closely related but different.
It's necessary to understand the difference so you can model the application and craft effective workloads.

{{< hint type=note >}}
These docs use "trx" only for Finch transactions, even though MySQL occasionally uses the abbreviation too.
{{< /hint >}}

{{< toc >}}

## Difference

**<mark>A Finch trx includes all SQL statments in _one file_.</mark>**
Here are three examples:

{{< columns >}}
_file-1.sql_
```sql
SELECT c FROM t WHERE id=?
```
<--->
_file-2.sql_
```sql
SELECT c FROM t WHERE id=?

UPDATE t SET n=1 WHERE c=?
```
<--->
_file-3.sql_
```sql
BEGIN 

SELECT c FROM t WHERE id=?

UPDATE t SET n=1 WHERE c=?

COMMIT
```
{{< /columns >}}

file-1.sql
: Finch trx: 1<br>
MySQL transactions: 1

file-2.sql
: Finch trx: 1<br>
MySQL transactions: 2 (when `autocommit=ON`)

file-3.sql
: Finch trx: 1<br>
MySQL transactions: 1 (explicit transaction)

No matter how many statements (or which statements) are in a Finch trx file, the file equals 1 Finch trx.
An extreme (and nonsensical) example is:

```sql
CREATE TABLE t (
  /* ... */
)

INSERT INTO t VALUES /* ... */

BEGIN

SELECT c FROM t /* ... */

UPDATE t SET /* ... */

COMMIT

DELETE FROM t /* ... */
```

That is 4 MySQL transactions: implicit commit on DLL (`CREATE`), `INSERT`, explicit transaction, `DELETE`.
But it's all 1 Finch trx.

## Modeling

Finch trx exist to encourage modeling benchmarks after real application transactions.
This allows better performance insight because Finch records stats per Finch trx.
(Although currently it doesn't _report_ them per trx; that's a todo feature.)

Moreover, Finch trx are related to MySQL transactions but different because this how real applications work.
For example, let's say your application has 3 important transactions that you want to benchmark:

{{< columns >}}
_Transaction A_
```sql
SELECT a1

SELECT a2
```
<--->
_Transaction B_
```sql
BEGIN

SELECT b1

UPDATE b2

COMMIT
```
<--->
_Transaction C_
```sql
UPDATE c1
```
{{< /columns >}}

Transaction A is actually 2 separate MySQL transactions (when `autocommit=ON`), but from the application point of view it's a single unit of work&mdash;a pseudo-transaction.
When benchmarking, developers need to know how both `SELECT` statements perform because they're inseparable in the application.
So while they're not 1 MySQL transaction, they are 1 Finch trx if put in the same Finch trx file.

_Without_ Finch trx (or in other benchmark tools), you usually wind up benchmarking the application transactions like:

```
cat A B C | benchmark
```

The benchmark executes everything as one "blob" workload.
One problem is: how do you make sense of the performance and stats?
If the benchmark reports 500 QPS and that's too low, which transaction is slowest?
Another problem is: what if you want to execute transaction A as fast as possible, but limit B and C to more closely reflect how the application works?

You can get creative and run the benchmark like:

```
cat A | benchmark &
cat B | benchmark &
cat C | benchmark &
```

That might work, but it's cumbersome and you miss the bigger picture: overall performance and stats.

Finch trx solve all these problems.
Write the application transactions in separate Finch trx file, then configure a workload like:

{{< highlight yaml "linenos=true" >}}
stage:
  workload:
    - clients: 16
      trx: [trx-A.sql]
      # As fast as possible

    - clients: 16
      trx: [trx-B.sql]
      qps-clients: 200

    - clients: 1
      trx: [trx-C.sql]
      qps: 10

  trx:
    - file: trx-A.sql
    - file: trx-B.sql
    - file: trx-C.sql
{{< /highlight >}}

Lines 3&ndash;5 create a client group of 16 clients to execute transaction A as fast as possible.
Lines 7&ndash;9 create a client group of 16 clients to execute transaction B limited to 200 QPS total (all clients).
Lines 11&ndash;13 create a client group of 1 client to execute transaction C at 10 QPS.

Lines 15&ndash;18 declare the Finch trx that compose the stage.
(Usually this section is longer, more complex.)

This is only an example to demonstrate how Finch trx should be used to model real application transactions.
[Benchmark / Workload]({{< relref "benchmark/workload" >}}) explains how to configure the `workload` section.

## TPS

Limit
: The transactions per second (TPS) limit works only with explicit `BEGIN` statements.
It does not apply to implicit transactions or Finch trx.

Stats
: The TPS stat measures only explicit `COMMIT` statements.
It does not apply to implicit transactions or Finch trx.
