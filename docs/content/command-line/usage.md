---
weight: 1
---

```sh
Usage:
  finch [options] STAGE_FILE [STAGE_FILE...]

Options:
  --bind ADDR[:PORT]    Listen for clients on ADDR, or "-" to disable server API (default: :33075)
  --debug               Print debug output to stderr
  --dsn                 MySQL DSN (overrides stage files)
  --help                Print help and exit
  --param (-p) KEY=VAL  Set param key=value (override stage files)
  --server ADDR[:PORT]  Connect to server at ADDR
  --test                Validate stages, test connections, and exit
  --version             Print version and exit

finch 1.0.0
```

You must specify at least one [stage file](/syntax/stage-file) on the commnand line.

Finch executes stages files in the order given.

## Command Line Options

### `--bind`

[Server](../client-server) addr:port to listen on for clients
{.tagline}

|Env Var|Default|Valid Value (n)|
|-----|-------|----|
|`FINCH_BIND`|:33075|ADDR[:PORT] or "-" to disable|
{.compact .params}

The default (no addr) binds to all interfaces on TCP port 33075.

`--bind "-"` disables the server.

<br>

### `--debug`

Print debug output to stderr.
{.tagline}

|Env Var|Default|Valid Value (n)|
|-----|-------|----|
|`FINCH_BIND`|:33075|ADDR[:PORT]|
{.compact .params}

Use with `--test` to debug startup validation, data geneartors, etc.

<br>

### `--dsn`

[Data source name (DSN)](https://github.com/go-sql-driver/mysql#dsn-data-source-name) for all MySQL connections
{.tagline}

|Env Var|Default|Valid Value (n)|
|-----|-------|----|
|`FINCH_DSN`||(See link below)|
{.compact .params}

This overrides any [`mysql`](/syntax/all-file/#mysql) settings.

<br>

### `--help`

Print help (the usage output above) and exit zero
{.tagline}

<br>

### `--param`

Set [params](/syntax/all-file/#params) that override all stage files
{.tagline}

This option can be specified multiple times:

```sh
finch --params key1=value1 --params key2=val2
```

<br>

### `--server`

Run Finch as a [client](../client-server/) connected to address and (optional) port
{.tagline}

|Env Var|Default|Valid Value (n)|
|-----|-------|----|
|`FINCH_SERVER`|||
{.compact .params}

<br>

### `--test`

Start up and validate everything possible, but don't execute any stages.
{.tagline}

|Env Var|Default|Valid Value (n)|
|-----|-------|----|
|`FINCH_TEST`||"1" to enable|
{.compact .params}

<br>

### `--version`

Print Finch version and exit zero.
{.tagline}
