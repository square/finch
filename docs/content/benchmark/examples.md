---
weight: 2
---

Finch ships with example benchmarks in subdirectory `benchmarks/`.
As complete working examples that range from simple to complex, they're good study material for learning how to write Finch benchmarks and use Finch features.

{{< toc >}}

## aurora

Original 2015 Amazon Aurora benchmark
{.tagline}

|Stage|Type|Description|
|-----|----|-----------|
|setup.yaml|DDL|Create schema and insert rows|
|write-only.yaml|Standard|Execute read-write transaction|

A short benchmark with a [long and complicated history](https://hackmysql.com/post/are-aurora-performance-claims-true/).
The short version: Amazon used this benchmark in 2015 to established its "5X greater throughput than MySQL" claim.
Under the hood, it's the sysbench write-only benchmark with a specific workload: 4 EC2 (compute) instances each running 1,000 clients (all in the same availability zone), querying 250 tables each with 25,0000 rows.

## intro

[Intro / Start Here]({{< relref "intro/start-here" >}}) benchmarks
{.tagline}

|Stage|Type|Description|
|-----|----|-----------|
|setup.yaml|DDL|Create schema and insert rows|
|read-only.yaml|Standard|Execute single SELECT|
|row-lock.yaml|Standard|Execute SELECT and UPDATE transaction on 1,000 rows|

## sysbench

sysbench benchmarks recreated in Finch
{.tagline}

|Stage|Type|Description|
|-----|----|-----------|
|setup.yaml|DDL|Create schema and insert rows|
|read-only.yaml|Standard|sysbench OLTP read-only benchmark|
|write-only.yaml|Standard|sysbench OLTP write-only benchmark|

These recreate the classic [sysbench](https://github.com/akopytov/sysbench) OLTP benchmarks: `oltp_read_only.lua` and `oltp_write_only.lua`.
They use one table name `sbtest1`.
Does not support multiple tables, but see the [aurora](#aurora) benchmark that is the same benchmark on 250 tables.

## xfer

Naïve money transfer (xfer) with three tables and large data set
{.tagline}

|Stage|Type|Description|
|-----|----|-----------|
|setup.yaml|DDL|Create schema and insert rows|
|xfer.yaml|Standard|Execute read-write transaction|

The xfer benchmark is complex and demonstrates many of the advanced features of Finch.
