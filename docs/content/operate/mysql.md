---
weight: 3
title: "MySQL"
---

{{< hint type=warning >}}
**Do not use Finch in production**. It's a development tool intended for use only in insecure development environments.
{{< /hint >}}

## User

Finch defaults to username `finch`, password `amazing` , host `127.0.0.1` (via TCP).

Finch is a development tool intended for use only in insecure development environments, but a safer MySQL user is suggested:

```sql
CREATE USER finch@'%' IDENTIFIED BY 'amazing';

GRANT SELECT, INSERT, UPDATE, DELETE, CREATE, DROP,  REFERENCES, INDEX, ALTER,  CREATE TEMPORARY TABLES, LOCK TABLES
ON finch.* TO finch@'%';
```

If you want to use your local root MySQL user, setting environment variable [`FINCH_DSN`]({{< relref "operate/command-line#--dsn" >}}) is the easiest way to override the defaults.

## Connection

Finch checks [`stage.mysql.dsn`]({{< relref "syntax/stage-file#mysql" >}}) first: if set, it's used and processing stops.
[`--dsn`]({{< relref "operate/command-line#--dsn" >}}) sets and overwrites `stage.mysql.dsn` from the command line: `stage.mysql.dsn = --dsn`.
If `stage.mysql.dsn` is not set, it inherits \_all.yaml [`mysql.dsn`]({{< relref "syntax/all-file#dsn" >}}).

If no DSN is specified, Finch loads the other [`stage.mysql`]({{< relref "syntax/stage-file#mysql" >}}) settings, which inherit values from \_all.yaml [`mysql`]({{< relref "syntax/all-file#mysql" >}}).

## Default Database

There are three ways to set the default database:

|Connection|Client Group|Trx|
|----------|------------|--------------|
|[Connection](#connection)|[`stage.workload[].db`]({{< relref "syntax/stage-file#db" >}})|`USE db` in a trx|

The connection default database is standard: the database that the client _requires_ when connecting to MySQL.
It's specified by [`--dsn`]({{< relref "operate/command-line#--dsn" >}}), or by [`--database`]({{< relref "operate/command-line#--database" >}}), or by [`mysql.db`]({{< relref "syntax/all-file#db" >}}).
If the database doesn't exist, the connection fails with an error:

```
% mysql -D does_NOT_exist
ERROR 1049 (42000): Unknown database 'does_not_exist'
```

The connection database is useful to ensure that all clients use it, but there's a problem: it doesn't work if the stage is going to create the database.

{{< hint type=warning >}}
It is **not recommended** to have a stage drop schemas or tables in a trx file.
A human should do this manually to ensure only the correct schemas or tables are dropped.
{{< /hint >}}

Although _not recommended_, a stage can create its own schema in a trx file like:

```SQL
DROP DATABASE IF EXISTS finch;

CREATE DATABASE finch;

USE finch;

CREATE TABLE ...
```

That's an example of a trx default database: using an explicit `USE db` statement, which Finch allows.

The client group default database is a special case: it make Finch execute `USE db` once for each client in the group during stage preparation (before the stage runs), which avoids having to use a trx default database.
This is done after connecting, so the database doesn't need to exist to connect, but it needs to exist when the client group is prepared (else [`--test`]({{< relref "operate/command-line#--test" >}}) will fail).
