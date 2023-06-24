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

