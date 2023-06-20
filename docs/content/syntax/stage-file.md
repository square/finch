---
weight: 2
---

Stage files are YAML format.
This is a quick reference with fake but syntactically valid values:

```yaml
stage:
  disable: false
  name: "read-only"
  qps: "1,000"
  runtime: "60s"
  tps: "500"
  
  compute:
    disable-local: false
    instances: 0

  mysql:
    # Override mysql from _all.yaml

  params:
    # Override params from _all.yaml

  stats:
    # Override stats from _all.yaml

  trx:
    - name: "foo" ##########
      file: "trx/foo.sql"  #
      data:                #
        d:                 #
          generator: "int" #
                           #
  workload:                #
    - trx: ["foo"] #########
      clients: 1
      db: ""
      iter: "0"
      iter-clients: "0"
      iter-exec-group: "0"
      qps: "0"
      qps-clients: "0"
      qps-exec-group: "0"
      runtime: "0s"
      tps: "0"
      tps-clients: "0"
      tps-exec-group: "0"
```

{{< toc >}}

## stage

A stage file starts with a top-level `stage:` declaration.

### disable

* Default: false
* Value: boolean

Disable the stage entirely if true.

### name

* Default: base file name
* Value: string

The stage name.

### qps

* Default: 0 (unlimited)
* Value: [string-int]({{< relref "syntax/values#string-int" >}}) &ge; 0

Queries per second (QPS) limit for all clients, all execution groups.

### runtime

* Default: 0 (unlimited)
* Value: [time duration]({{< relref "syntax/values#time-duration" >}}) &ge; 0

How long to run the stage.
If zero and there are no [data limits]({{< relref "data/limits" >}}), use CTRL-C to stop the stage and report stats.

### tps

* Default: 0 (unlimited)
* Value: [string-int]({{< relref "syntax/values#string-int" >}}) &ge; 0

Transaction per second (TPS) limit for all clients, all execution groups.

---

## compute

### disable-local

* Default: false
* Value: boolean

If true, the local Finch instances does not count as 1 compute.

### instances

* Default: 1
* Value: [string-int]({{< relref "syntax/values#string-int" >}}) &ge; 0

The number of compute instances that Finch requires to run the benchmark.

---

## mysql

See [`mysql` in _all.yaml_]({{< relref "syntax/all-file#mysql" >}}).

---

## params

See [`params` in _all.yaml_]({{< relref "syntax/all-file#params" >}}).

---

## stats

See [`stats` in _all.yaml_]({{< relref "syntax/all-file#stats" >}}).


---

## trx

The `trx` section is a required _list_ of Finch [trx files]({{< relref "syntax/trx-file" >}}) to load.

```yaml
stage:
  trx:
    - file: trx/A.sql
      name: foo

    - file: trx/B.sql
      # name defaults to B.sql
```

### file

* Default: (none; must be set explicitly)
* Value: file name

Sets one Finch trx file to load.
Paths are relative to the directory of the stage file.

### data

* Default: (none; must be set explicitly)
* Value: map keyed on data key name without `@` prefix

Every [data key]({{< relref "data/keys" >}}) in a trx file must be defined in the stage file.

```yaml
stage:
  trx:
    - file: foo.sql         # uses @d
      data:
        d:                  # key for @d
          generator: "int"  # __REQUIRED__
          scope: "trx"      # optional
          params:           # generator-specific
            max: "10,000"   #
```

Only `generator` is required, but you usually need to specify `params` too.

{{< hint type=note >}}
Only @d ("d" in the stage file) is shown here, but every data key in a trx file must be defined in the `data` map.
{{< /hint >}}

#### d.data-type

* Default: "n"
* Value: "n" or "s"

Indicates column type: "n" for a numeric value, or "s" for a string (quoted) value.

#### d.generator

* Default: (none; must be set explicitly)
* Type: name of [data generator]({{< relref "data/generators" >}})

Name of the data generator to use for the data key.

#### d.params

* Default: (none)
* Value: key-value map of strings

Generator-specific params.
These are optional but usually needed.
See [Data / Generators]({{< relref "data/generators" >}})

#### d.scope

* Default: statement
* Value: "" or "statement", "trx", "client"

Scope of data generator.
If not specified (empty string), the default is "statement".
See [Data / Scope]({{< relref "data/scope" >}}).

### name

* Default: Base file name
* Value: any string

Set trx name used in [`workload.trx`](#trx-1) list.

## workload

The `workload` section declares the [workload]({{< relref "benchmark/workload" >}}) that references the [`trx`](#trx) section.

### clients

* Default: 1
* Value: [string-int]({{< relref "syntax/values#string-int" >}}) &ge; 1

Number of clients to run in client group.

### iter

### iter-clients

### iter-exec-group

* Default: 0 (unlimited)
* Value: [string-int]({{< relref "syntax/values#string-int" >}}) &ge; 1

Maximum number of iterations to execute per client, client group, or execution group (respectively).

### qps

### qps-clients

### qps-exec-group

* Default: 0 (unlimited)
* Value: [string-int]({{< relref "syntax/values#string-int" >}}) &ge; 1

Maximum rate of queries per second (QPS) per client, client group, or execution group (respectively).

### runtime

* Default: 0 (forever)
* Value: [string-int]({{< relref "syntax/values#string-int" >}}) &ge; 1

Runtime limit

### tps

### tps-clients

### tps-exec-group

* Default: 0 (unlimited)
* Value: [string-int]({{< relref "syntax/values#string-int" >}}) &ge; 1

Maximum rate of transaction per second (TPS) per client, client group, or execution group (respectively).

### trx

* Default: none or auto
* Value: list of [`trx.name`](#name-1)

Trx assigned to all clients to execute.
