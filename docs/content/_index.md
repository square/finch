---
weight: 0
---

# Finch

Finch is a MySQL benchmark tool.
Like most benchmark tools, it executes SQL statements, measures response time, and reports various statistics (QPS, min, max, P95).
But _unlike_ most benchmark tools, Finch was developed for software engineers and modern infrastructures.

## 💡 Declarative

Write benchmarks in SQL, not a scripting language.

Let's say you're a developer who wants to know how the following transaction will perform on your database:

```sql
BEGIN

SELECT id FROM t WHERE user=?

UPDATE t SET n=n+1 WHERE id=?

COMMIT
```

That's how you write the benchmark with Finch: with SQL.
This allows developers (and DBAs) to write benchmarks easily&mdash;without knowing an additional scripting language or programming API.

## 💡 Workload Orchestration

Normal benchmark tools execute all transactions, in all clients, all at once.

![Normal workload execution](/finch/img/normal_benchmark_workload.svg)

But real workloads (the combination of queries, data, and access patterns) can be far more complex.
Finch lets you orchestrate complex workloads.

![Finch workload shaping](/finch/img/finch_benchmark_workload.svg)

In the diagram above, clients in client group 1 execute statements in transaction 3 while, concurrently, clients in client group 2 execute statements in transaction 2.
Together, these client groups form execution group 1.
When execution group 1 finishes, execution group 2 begins.
Clients in client group 3 execute statements in transactions 2 and 1.

Whether your benchmark workload is simple or complex, Finch can execute it.

## 💡 Distributed Compute

A single Finch server can coordinate multiple Finch clients running on separate compute instances.

![Finch Distributed Compute](/finch/img/finch_compute.svg)

This allows you to hammer the database like the application: from any number of compute instances.
It's a benchmarking bot network&mdash;wield the power responsibly.

## 💡 Plug-in Stats

Finch stats are reported through plugins.
Don't like CSV?
Write a little Go plugin and dump the Finch stats anywhere, anyway you want.
Finch doesn't judge; it just works hard.

---

{{< hint type=tip title="Read Next" >}}
[Start Here]({{< relref "intro/start-here" >}}) to learn Finch by writing quick and simple benchmarks.
{{< /hint >}}
