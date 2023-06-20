---
weight: 1
---

See and learn how Finch works by creating and running simple benchmarks.
Follow this page from top to bottom &darr;

## One-time Setup

Compile the `finch` binary:

```bash
cd bin/finch/
go build finch
```

_Stay in this directory._

Create a MySQL user, database, and table that Finch can use. Running the following as a MySQL root/super user.

{{< hint type=tip >}}
To copy, mouse over a code block and click the button in the upper-right.
{{< /hint >}}

```sql
CREATE USER finch@'%' IDENTIFIED BY 'amazing';

GRANT ALL ON finch.* TO finch@'%';
```

```sql
CREATE DATABSASE IF NOT EXISTS finch;

USE finch;

DROP TABLE IF EXISTS t1;

CREATE TABLE t1 (
  id INT UNSIGNED NOT NULL PRIMARY KEY,
  n  INT NOT NULL,
  c  VARCHAR(100) NOT NULL,
  INDEX (n)
);
```

## Benchmarks

### INSERT

Copy-paste each code block into the file name given above each one.

{{< columns >}}
_insert-rows.sql_
```sql
-- rows: 100,000
INSERT INTO finch.t1 (id, n, c) VALUES (@id, @n, @c)
```
<---> <!-- magic separator, between columns -->
_setup.yaml_
```yaml
stage:
  trx:
    - file: insert-rows.sql
      data:
        id:
          generator: "auto-inc"
        n:
          generator: "int"
        c:
          generator: "str-fill-az"
```

{{< /columns >}}

Run the INSERT benchmark:

```bash
./finch --dsn 'finch:amazing@tcp(127.0.0.1)/' setup.yaml
```

Finch should complete after few seconds with output similar to below (click to expand).

{{< expand "Finch Output">}}
```none
2023/06/04 16:28:17.316097 boot.go:59: map[CPU_CORES:8]
#
# setup.yaml
#
2023/06/04 16:28:17.316996 factory.go:54: finch:...@tcp(127.0.0.1:3306)/
2023/06/04 16:28:17.317980 stage.go:61: Connected to finch:...@tcp(127.0.0.1:3306)/
2023/06/04 16:28:17.318356 server.go:131: Have 0 instances, need 1
2023/06/04 16:28:17.318385 server.go:139: local booted
2023/06/04 16:28:17.318440 stage.go:135: [setup.yaml] Running (no runtime limit)
2023/06/04 16:28:17.318457 stage.go:147: [setup.yaml] Execution group 1, client group 1, runnning 1 clients
2023/06/04 16:28:17.632753 data.go:99: 501 / 10,000 = 5.0% in 0s: 1,597 rows/s (ETA 5s)
2023/06/04 16:28:17.928739 data.go:99: 1,002 / 10,000 = 10.0% in 0s: 1,692 rows/s (ETA 5s)
2023/06/04 16:28:18.229864 data.go:99: 1,503 / 10,000 = 15.0% in 0s: 1,663 rows/s (ETA 5s)
2023/06/04 16:28:18.527114 data.go:99: 2,003 / 10,000 = 20.0% in 0s: 1,682 rows/s (ETA 4s)
2023/06/04 16:28:18.817552 data.go:99: 2,504 / 10,000 = 25.0% in 0s: 1,725 rows/s (ETA 4s)
2023/06/04 16:28:19.109760 data.go:99: 3,005 / 10,000 = 30.0% in 0s: 1,714 rows/s (ETA 4s)
2023/06/04 16:28:19.404558 data.go:99: 3,506 / 10,000 = 35.1% in 0s: 1,699 rows/s (ETA 3s)
2023/06/04 16:28:19.718560 data.go:99: 4,007 / 10,000 = 40.1% in 0s: 1,595 rows/s (ETA 3s)
2023/06/04 16:28:20.020593 data.go:99: 4,508 / 10,000 = 45.1% in 0s: 1,658 rows/s (ETA 3s)
2023/06/04 16:28:20.305606 data.go:99: 5,008 / 10,000 = 50.1% in 0s: 1,754 rows/s (ETA 2s)
2023/06/04 16:28:20.597114 data.go:99: 5,509 / 10,000 = 55.1% in 0s: 1,718 rows/s (ETA 2s)
2023/06/04 16:28:20.857359 data.go:99: 6,010 / 10,000 = 60.1% in 0s: 1,925 rows/s (ETA 2s)
2023/06/04 16:28:21.097476 data.go:99: 6,510 / 10,000 = 65.1% in 0s: 2,082 rows/s (ETA 1s)
2023/06/04 16:28:21.327148 data.go:99: 7,011 / 10,000 = 70.1% in 0s: 2,181 rows/s (ETA 1s)
2023/06/04 16:28:21.592082 data.go:99: 7,512 / 10,000 = 75.1% in 0s: 1,891 rows/s (ETA 1s)
2023/06/04 16:28:21.874768 data.go:99: 8,013 / 10,000 = 80.1% in 0s: 1,772 rows/s (ETA 1s)
2023/06/04 16:28:22.167851 data.go:99: 8,514 / 10,000 = 85.1% in 0s: 1,709 rows/s (ETA 0s)
2023/06/04 16:28:22.445353 data.go:99: 9,015 / 10,000 = 90.1% in 0s: 1,805 rows/s (ETA 0s)
2023/06/04 16:28:22.691533 data.go:99: 9,515 / 10,000 = 95.2% in 0s: 2,031 rows/s (ETA 0s)
2023/06/04 16:28:22.968082 stage.go:178: [setup.yaml] Client done
 interval| duration| runtime| clients|   QPS| min|  P999|   max| r_QPS| r_min| r_P999| r_max| w_QPS| w_min| w_P999| w_max| TPS| c_min| c_P999| c_max| errors|compute
        1|        6|       5|       1| 1,770| 255| 1,513| 8,091|     0|     0|      0|     0| 1,770|   255|  1,513| 8,091|   0|     0|      0|     0|      0|local

2023/06/04 16:28:22.969152 server.go:176: local completed stage setup.yaml
2023/06/04 16:28:22.969174 server.go:178: 0/1 instances running
```
{{< /expand >}}

The benchmark statistics are printed near the end of the output like (scroll right &rarr;):

```
 interval| duration| runtime| clients|   QPS| min|  P999|   max| r_QPS| r_min| r_P999| r_max| w_QPS| w_min| w_P999| w_max| TPS| c_min| c_P999| c_max| errors|compute
        1|        6|       5|       1| 1,770| 255| 1,513| 8,091|     0|     0|      0|     0| 1,770|   255|  1,513| 8,091|   0|     0|      0|     0|      0|local
```

Your numbers will vary (possible by a lot).
Since this is a quick tutorial, let's quickly examine columns:

``` 
|   QPS| min|  P999|   max|
| 1,770| 255| 1,513| 8,091|
```

These columns include all statements, but since this is an INSERT-only benchmark, all numbers reflect the INSERT statement.
The `r_` columns are stats for reads (`SELECT`).
The `w_` columns are stats for writes.
The `c_` columns are stats for `COMMIT`.

Finch reports all query times in microseconds (&micro;s).

Everyone knows QPS: 1,770 on this run.

The minimum query response time was 255 &micro;s.
The compute has a locally-attached SSD, so this minimum is believable.

The P999 (99.9th percentile) query response time was about 1.5 _milliseconds_ (converting from &micro;s).

The maximum query response time was about 8.1 milliseconds (converting from &micro;s), which is pretty high for a locally-attached SSD.

### Read-only

Copy-paste each code block into the file name given above each one.

{{< columns >}}
_read-only.sql_
```sql
SELECT n, c FROM finch.t1 WHERE id = @id
```
<---> <!-- magic separator, between columns -->
_read-only.yaml_
```yaml
stage:
  runtime: 10s
  workload:
    - clients: 4
  trx:
    - file: read-only.sql
      data:
        id:
          generator: "int"
```
{{< /columns >}}

Run the read-only benchmark:

```bash
./finch --dsn 'finch:amazing@tcp(127.0.0.1)/' read-only.yaml
```

What kind of QPS and response time stats did you get on your machine?
On this machine:

```
 interval| duration| runtime| clients|    QPS| min| P999|   max|  r_QPS| r_min| r_P999| r_max| w_QPS| w_min| w_P999| w_max| TPS| c_min| c_P999| c_max| errors|compute
        1|       10|      10|       4| 17,651|  47|  794| 2,406| 17,651|    47|    794| 2,406|     0|     0|      0|     0|   0|     0|      0|     0|      0|local
```

17,651 QPS: not bad. A 47 microsecond read: very fast. A 2.4 millisecond read: pretty slow.

### Row Lock Contention

The previous two benchmarks were too easy.
Let's write a transaction (trx) that locks and update rows, then run that trx on several clients.
To make it extra challenging, let's limit it to the first 1,000 rows.
This should create row lock contention and slow down performance noticeably.

Copy-paste each code block into the file name given above each one.

{{< columns >}}
_rw-trx.sql_
```sql
BEGIN

SELECT c FROM finch.t1 WHERE id = @id FOR UPDATE

UPDATE finch.t1 SET n = n + 1 WHERE id = @id

COMMIT
```
<---> <!-- magic separator, between columns -->
_row-lock.yaml_
```yaml
stage:
  runtime: 20s
  workload:
    - clients: 4
  trx:
    - file: rw-trx.sql
      data:
        id:
          generator: "int"
          scope: trx
          params:
            max: 1,000
```
{{< /columns >}}

Before running the benchmark, execute this query on MySQL to get the current number of row lock waits:

```sql
SHOW GLOBAL STATUS LIKE 'Innodb_row_lock_waits';
```

Then run the slow row-lock benchmark:

```bash
./finch --dsn 'finch:amazing@tcp(127.0.0.1)/' row-lock.yaml
```

Once the benchmark completes (in 20s), execute `SHOW GLOBAL STATUS LIKE 'Innodb_row_lock_waits';` on MySQL again.
The number of row locks waits should have increased, proving that the trx caused them.

Query response times should be higher (slower), too:

```
 interval| duration| runtime| clients|   QPS| min|  P999|    max| r_QPS| r_min| r_P999|  r_max| w_QPS| w_min| w_P999|  w_max|   TPS| c_min| c_P999|  c_max| errors|compute
        1|       20|      20|       4| 9,461|  80| 1,659| 79,518| 2,365|   148|  1,096| 37,598| 2,365|   184|  1,202| 40,770| 2,365|   366|  2,398| 79,518|      0|local
```

Scroll right (&rarr;) and notice `TPS 2,365`: Finch measures transactions per second (TPS) when the Finch trx (_rw-trx.sql_) has an explicit SQL transaction: `BEGIN` and `COMMIT`.

`c_max 79,518` means the maximum `COMMIT` time was 79,518 &micro;s (80 milliseconds), which is pretty slow for a locally-attached SSD, but this is why Finch measures `COMMIT` latency: when a transaction commits, MySQL makes the data changes durable on disk, which is one of the slowest (but most important) operations of an ACID-compliant database.

---

{{< hint type=tip title="Read Next" >}}
Learn the [Concepts](/intro/concepts) underlying these benchmarks and how Finch works.
{{< /hint >}}

