---
weight: 1
---

```sh
Usage:
  finch [options] STAGE_FILE [STAGE_FILE...]

Options:
  --client ADDR[:PORT]  Run as client of server at ADDR
  --cpu-profile FILE    Save CPU profile of stage execution to FILE
  --database (-D) DB    Default database on connect
  --debug               Print debug output to stderr
  --dsn DSN             MySQL DSN (overrides stage files)
  --help                Print help and exit
  --param (-p) KEY=VAL  Set param key=value (override stage files)
  --server ADDR[:PORT]  Run as server on ADDR
  --test                Validate stages, test connections, and exit
  --version             Print version and exit

finch 1.0.0
```

You must specify at least one [stage file]({{< relref "syntax/stage-file" >}}) on the command line.

Finch executes stages files in the order given.

## Command Line Options

### `--client`

Run as [client]({{< relref "operate/client-server" >}}) connected to address and (optional) port.
{.tagline}

|Env Var|Value|Default|Valid Value|
|-------|-----|-------|-----------|
|`FINCH_CLIENT`|ADDR[:PORT]||Server address[:port]|
{.compact .params}

<br>

### `--cpu-profile`

Save CPU profile of stage execution.
{.tagline}

|Env Var|Value|Default|Valid Value|
|-------|-----|-------|-----------|
|`FINCH_CPU_PROFILE`|FILE||file name|
{.compact .params}

For example, `--cpu-profile cpu.prof` then run `go tool pprof -http 127.1:8080 ./finch cpu.prof`.

<br>

### `--database`

|Env Var|Value|Default|Valid Value|
|-------|-----|-------|-----------|
|`FINCH_DB`|DB||Database name|
{.compact .params}

Sets [default database]({{< relref "operate/mysql#default-database" >}}) on connect.
Does not override [`--dsn`](#--dsn`).

<br>

### `--debug`

Print debug output to stderr.
{.tagline}

|Env Var|
|-------|
|`FINCH_DEBUG`|
{.compact .params}

Use with `--test` to debug startup validation, data generators, etc.

<br>

### `--dsn`

Data source name (DSN) for all MySQL connections.
{.tagline}

|Env Var|Value|Default|Valid Value|
|-------|-----|-------|-----------|
|`FINCH_DSN`|DSN||[Data Source Name](https://github.com/go-sql-driver/mysql#dsn-data-source-name)|
{.compact .params}

This overrides any [`mysql`]({{< relref "syntax/all-file#mysql" >}}) settings.

There's no default DSN, but there is a default [MySQL user]({{< relref "operate/mysql#user" >}}).

<br>

### `--help`

Print help (the usage output above) and exit zero.
{.tagline}

<br>

### `--param`

Set [params]({{< relref "syntax/all-file#params" >}}) that override all stage files.
{.tagline}

This option can be specified multiple times:

```sh
finch --params key1=value1 --params key2=val2
```

<br>

### `--server`

Run as [server]({{< relref "operate/client-server" >}}) on addr:port to listen on for clients.
{.tagline}

|Env Var|Value|Default|Valid Value|
|-------|-----|-------|-----------|
|`FINCH_SERVER`|ADDR[:PORT]||Interface address and port|
{.compact .params}

Using value "0" or "0.0.0.0" will bind to all interfaces on port 33075.

<br>

### `--test`

Start up and validate everything possible, but don't execute any stages.
{.tagline}

|Env Var|
|-------|
|`FINCH_TEST`|
{.compact .params}

Use with [`--debug`](#--debug) to debug file syntax, workload allocation, and so forth.
Finch start up is two-phase: prepare and run.
This option makes Finch exit after prepare.
Finch prepares everything in the prepare phase, so a successful test means that the Finch configuration is valid.
The only thing Finch does not check during prepare is that the SQL statements are valid.
That happens in the run phase when Finch executes the SQL statements.
For example, Finch doesn't check if "SELECT c FRM t WHRE id=1" is valid; it relies on MySQL to parse and validate SQL statements.
However, statements with the [prepare SQL modifier]({{< relref "syntax/trx-file#prepare" >}}) are prepared on MySQL during the Finch prepare phase, so invalid prepared SQL statements will cause an error during a Finch startup test.

<br>

### `--version`

Print Finch version and exit zero.
{.tagline}
