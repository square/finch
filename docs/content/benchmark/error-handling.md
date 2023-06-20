---
weight: 6
---

Finch handles these MySQL errors _without_ reconnecting:

|Error|MySQL Error Code|Handling|
|-----|----------------|--------|
|Deadlock|1213|MySQL automatically rolls back|
|Lock wait timeout|1205|Execute `ROLLBACK` because `innodb_rollback_on_timeout=OFF` by default|
|Query killed|1317||
|Read-only|1290, 1836||
|Duplicate key|1062||

After handling the errors above, Finch starts a new iteration from the first [assigned trx]({{< relref "benchmark/workload#trx" >}}).

Other errors cause Finch to disconnect and reconnect to MySQL, then start a new iteration.
Reconnect time is not directly measured or recorded, but if it's severe it will reduce reported throughput because Finch will spend time reconnecting rather than executing queries.

Query [statistics]({{< relref "benchmark/statistics" >}}) are recorded when the query returns an error.
This is usually correct because, for example, a lock wait timeout is part of query response time.
However, for errors that cause a fast error-retry-error loop, it will skew statistics towards zero or artificially high values.
