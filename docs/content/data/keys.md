---
weight: 1
---

_Data keys_ are placeholders in SQL statements, like "@d", that are replaced with real values when executed by a client.

![Finch data key and generator](/img/finch_data_key.svg)

Data keys are used in [trx files](/syntax/trx-file/), configured in [stage files](/syntax/stage-file/), and are replaced at runtime when executed by a client.

{{< toc >}}

## Name and Configure

The naming rule is simple: "@" followed by a single word (no spaces) in trx files/SQL statements, but no "@" prefix in stage files:

{{< columns >}}
_Use in Trx File/SQL Statement_
```
@d
@id
@last_updated
```
<--->
_Configure in Stage File_
```
d
id
last_updated

```
{{< /columns >}}

Examples in these docs use the canonical data key (@d) and other short names, but a good practice is to name data keys after the columns for which they're used:

```
SELECT c FROM t WHERE id = @id AND n > @n AND @k IS NOT NULL
```

Data keys are configured in [`stage.trx.[].data`](/syntax/stage-file/#data): each data key must have a corresponding entry in that map.
Since "@" is a special character in YAML, the data key names in a stage file do _not_ have the "@" prefix:

```yaml
stage:
  trx:
    - file: read.sql
      data:
        d:                   # @d
          generator: ing
        id:                  # @id
          generator: int
          params:
            max: 4294967296
        last_updated:        # @last_updated
          generator: int
```

The only required configuration is `generator`: the name of a [data generator](../generators/) to use for the data key.
But you most likely need to specify generator-specific parameters, as shown above for "id".
See [`stage.trx.[].data`](/syntax/stage-file/#data) for the full configuration.

{{< hint type=note title="Terminology" >}}
The terms "data key" and "data generator" are used interchangeably because they're two sides of the same coin.
A specific term is used when necessary to make a technical distinction. 
{{< /hint >}}

## Duplicates

You can reuse the same data key name in a trx file, like @id shown below.

```
SELECT c FROM t WHERE id = @id

UPDATE t SET n=n+1 WHERE id = @id
```

This works because the default data scope is statement: Finch creates one data generator for @id in the `SELECT`, and another for @id in the `UPDATE`.
However, there are two important points to consider:

* @id in both statments will have the same configuration because they'll be the same map key ("id") in [`stage.trx.[].data`](/syntax/stage-file/#data)&mdash;same name, same configuration.
* _[Data scope](../scope/)_ determines if the two @id are different or the same, and "when": if they're different, when do they generate differnet values?

{{< hint type=warning title="Data Scope" >}}
Be sure to read [Data / Scope](../scope/) to fully understand the second point.
{{< /hint >}}

The second point&mdash;[data scope](../scope/)&mdash;is impotant because, in that trx, it looks like the two statements are supposed to access the same row (read the row, then update it).
If that's true, then the default statememnt data scope won't do what you want.
Instead, you need to make @id the _same_ in both statements by giving it trx scope:

```yaml
stage:
  trx:
    - file: read.sql
      data:
        id:
          scope: trx # <-- data scope
          generator: int
```

Now the generator will return the same value for @id wherever @id is used in the trx.

## Meta

Meta data keys don't directly generate a value but serve another purpose as documented.
You do _not_ configure meta data keys in a stage file.

### @PREV

@PREV refers to the previous data key.
It's required for ranges: `BETWEEN @d AND @PREV`.
For example:

```yaml
stage:
  trx:
    - file: read.sql
      data:
        d:
          generator: int-range
```

The [int-range generator](/data/generators/#int-range) returns two values (an ordered pair): the first value repalced @d, the second value replaces @PREV.

