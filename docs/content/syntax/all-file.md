---
title: "_all.yaml"
weight: 3
---

The optional all stages file \_all.yaml is YAML format and applies to all stages in the same directory.
It's like a MySQL defaults file (my.cnf): it provides a default configuration that stage files in the same directory inherit but can also override.

\_all.yaml is _not_ a stage file.
There is no top-level `stage` section.
The only valid top-level sections in \_all.yaml are `mysql`, `parameters`, and `stats`.
These three sections can be specified in a [stage file](/syntax/stage-file/) to override \_all.yaml.

This is a quick reference with fake but syntactically valid values:

```yaml
mysql:
  db: ""
  dsn: ""
  hostname: ""
  mycnf: ""
  password: ""
  password-file: ""
  socket: ""
  timeout-connect: "10s"
  username: ""

  disable-auto-tls: false

  tls:

parameters:
  key1: "value1"
  keyN: "valueN"

stats:
  disable: false
  freq: "5s"
  report:
    csv:
      percentiles: "P95,P99"
      # More csv reporter params
    stdout:
      percentiles: "P999"
      # More stdout reporter params
```

{{< toc >}}

## mysql

The `mysql` section configures the connection to MySQL for all clients.

### db

Default datbase.

### dsn

Data source.

### hostname

Hostname of MySQL.

### mycnf

my.cnf to read default MySQL configuraiton.

### password

MySQL user password.

### password-file

File to read MySQL user password from.

### socket

MySQL socket.

### timeout-connect
* Default: 10s
* Value: [time duration](../values/#time-duration) &ge; 0

Timeout on connecting to MySQL.

### username

MySQL username

---

## params

The `params` section is an optional key-value map of strings.
See [Intro / Concepts / Parameters](/intro/concepts/#parameters) and [Syntax / Params](/syntax/params/).

---

## stats

The `stats` section configure statistics collection and reporting.
By default, Finch prints [statistics](/benchmark/statistics/) once, to stdout, when the stage completes. 
Different reporters can be used at the same time, but only one instance of each reporter.

### disable

* Default: false
* Value: boolean

Disable stats.
For example, you can enable and configure stats once in \_all.yaml for all stages, but disalbe stats in a [DDL stage](/benchmark/overview/#ddl) by specifying:

```yaml
# DDL stage
stage:
  stats:
    disable: true
```

### freq

* Default: 0 (disabled)
* Value: [time duration](../values/#time-duration) &ge; 0

How frequently to report periodic stats for all reporters.
(Frequency per reporter is not supported.)
If disabled, only one final report is printed at the end of the stage.

See [Benchmark / Statistics / Frequency](/benchmark/statistics/#frequency).

### report

The `report` section is a map keyed on reporter name ([built-in](/benchmark/statistics/#reporters) and [custom](/api/stats)).
The value of each map is a key-value map of reporter-specific parameters.

```yaml
stats:
  report:
    stdout:
      percentiles: "P95,P99"
    csv:
      percentiles: "P999"
```

See [Benchmark / Statistics / Reporters](/benchmark/statistics/#reporters) for `stdout` and `cvs` parameters.
