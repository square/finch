---
weight: 3
title: "TL;DR"
---

* Finch benchmarks MySQL; other databases are not currently supported.
* Benchmarks are written in SQL; write your queries in a `.sql` file.
* Each SQL file is a [Finch trx](/syntax/trx-file/)&mdash;"trx" for short.
* Trx should [model](/benchmark/trx/) real MySQL transactions, but they don't have to.
* In SQL statements, [data keys](/data/keys/) like @d inject fake/random/test data.
* Data keys are conifgured in [stage files](/syntax/stage-file/) to use a [data geneator](/data/generators/).
* Data generators are plugins that generate data (e.g. random numbers).
* At least one stage file is required to configure and run Finch.
* A benchmark comprises one or more stages (defined by stage files).
* After writing SQL files with data keys, write a stage file to configure and execute.
* [Statistics](/benchmark/statistics) are reported once at the end by default; periodic stats can be enabled.
* Stats reporters are plugins, so you can report stats any way, any where.
* Look in `benchmarks/sysbench/` for a complete and familar example.
