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

{{< hint type=tip title="Click to Copy" >}}
To copy, mouse over a code block and click the button in the upper-right.
{{< /hint >}}

```sql
CREATE USER finch@'%' IDENTIFIED BY 'amazing';

GRANT SELECT, INSERT, UPDATE, DELETE, CREATE, DROP,  REFERENCES, INDEX, ALTER,  CREATE TEMPORARY TABLES, LOCK TABLES
ON finch.* TO finch@'%';
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
2023/06/24 15:58:02.966170 boot.go:59: map[CPU_CORES:8]
#
# setup.yaml
#
2023/06/24 15:58:02.969392 factory.go:51: finch:...@tcp(127.0.0.1:3306)/?parseTime=true
2023/06/24 15:58:02.972926 stage.go:62: Connected to finch:...@tcp(127.0.0.1:3306)/?parseTime=true
2023/06/24 15:58:02.974299 stage.go:136: [setup.yaml] Running (no runtime limit)
2023/06/24 15:58:02.974327 stage.go:152: [setup.yaml] Execution group 1, client group 1, runnning 1 clients
2023/06/24 15:58:05.980739 data.go:99: 5,001 / 100,000 = 5.0% in 3s: 1,664 rows/s (ETA 57s)
2023/06/24 15:58:08.948397 data.go:99: 10,002 / 100,000 = 10.0% in 3s: 1,685 rows/s (ETA 53s)
2023/06/24 15:58:12.006452 data.go:99: 15,003 / 100,000 = 15.0% in 3s: 1,635 rows/s (ETA 51s)
2023/06/24 15:58:15.203869 data.go:99: 20,004 / 100,000 = 20.0% in 3s: 1,564 rows/s (ETA 51s)
2023/06/24 15:58:18.389031 data.go:99: 25,005 / 100,000 = 25.0% in 3s: 1,570 rows/s (ETA 47s)
2023/06/24 15:58:21.798925 data.go:99: 30,006 / 100,000 = 30.0% in 3s: 1,466 rows/s (ETA 47s)
2023/06/24 15:58:25.538797 data.go:99: 35,007 / 100,000 = 35.0% in 4s: 1,337 rows/s (ETA 48s)
2023/06/24 15:58:28.754100 data.go:99: 40,008 / 100,000 = 40.0% in 3s: 1,555 rows/s (ETA 38s)
2023/06/24 15:58:32.253534 data.go:99: 45,009 / 100,000 = 45.0% in 3s: 1,429 rows/s (ETA 38s)
2023/06/24 15:58:35.352258 data.go:99: 50,010 / 100,000 = 50.0% in 3s: 1,613 rows/s (ETA 30s)
2023/06/24 15:58:38.187871 data.go:99: 55,010 / 100,000 = 55.0% in 3s: 1,763 rows/s (ETA 25s)
2023/06/24 15:58:41.115210 data.go:99: 60,011 / 100,000 = 60.0% in 3s: 1,708 rows/s (ETA 23s)
2023/06/24 15:58:44.081488 data.go:99: 65,012 / 100,000 = 65.0% in 3s: 1,685 rows/s (ETA 20s)
2023/06/24 15:58:46.834091 data.go:99: 70,013 / 100,000 = 70.0% in 3s: 1,816 rows/s (ETA 16s)
2023/06/24 15:58:49.852387 data.go:99: 75,014 / 100,000 = 75.0% in 3s: 1,656 rows/s (ETA 15s)
2023/06/24 15:58:52.619915 data.go:99: 80,015 / 100,000 = 80.0% in 3s: 1,807 rows/s (ETA 11s)
2023/06/24 15:58:55.257472 data.go:99: 85,016 / 100,000 = 85.0% in 3s: 1,896 rows/s (ETA 7s)
2023/06/24 15:58:58.177528 data.go:99: 90,017 / 100,000 = 90.0% in 3s: 1,712 rows/s (ETA 5s)
2023/06/24 15:59:01.059509 data.go:99: 95,018 / 100,000 = 95.0% in 3s: 1,735 rows/s (ETA 2s)
 interval| duration| runtime| clients|   QPS| min|  P999|     max| r_QPS| r_min| r_P999| r_max| w_QPS| w_min| w_P999|   w_max| TPS| c_min| c_P999| c_max| errors|compute
        1|     60.8|    60.8|       1| 1,645| 365| 3,019| 192,360|     0|     0|      0|     0| 1,645|   365|  3,019| 192,360|   0|     0|      0|     0|      0|local

2023/06/24 15:59:03.737413 stage.go:206: [setup.yaml] Stage done because clients completed
```
{{< /expand >}}

The benchmark statistics are printed near the end of the output like (scroll right &rarr;):

```
 interval| duration| runtime| clients|   QPS| min|  P999|     max| r_QPS| r_min| r_P999| r_max| w_QPS| w_min| w_P999|   w_max| TPS| c_min| c_P999| c_max| errors|compute
        1|     60.8|    60.8|       1| 1,645| 365| 3,019| 192,360|     0|     0|      0|     0| 1,645|   365|  3,019| 192,360|   0|     0|      0|     0|      0|local
```

Your numbers will vary (possible by a lot).
Since this is a quick tutorial, let's just examine these four columns that are stats for all queries:

``` 
   QPS| min|  P999|     max|
 1,645| 365| 3,019| 192,360|
```

Everyone knows QPS: 1,645 on this run.
The next three columns are query response times, and Finch reports all query times in microseconds (&micro;s).

* The minimum query response time was 365 &micro;s.
The compute has a locally-attached SSD, so this minimum is believable.
* The P999 (99.9th percentile) query response time was about 3.0 _milliseconds_ (converting from &micro;s).
* The maximum query response time was about 192 milliseconds (converting from &micro;s), which is high for a locally-attached SSD.

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
Learn the [Concepts]({{< relref "intro/concepts" >}}) underlying these benchmarks and how Finch works.
{{< /hint >}}

