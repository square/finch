---
weight: 3
---

{{< toc >}}

## Integer

All integers are `int64` unless otherwise noted.

### int

Random integer between `[min, max]` with uniform or normal distribution
{.tagline}

|Param|Default|Valid Values (v)|
|-----|-------|----|
|`min`|1|v &ge; 0|
|`max`|100,000|v &lt; 2<sup>64</sup>|
|`dist`|`uniform`|`uniform` or `normal`|
{.compact .params}

### int-gaps

`p` percentage of integers between `[min, max]` with uniform random access
{.tagline}

|Param|Default|Valid Values (n)|
|-----|-------|----|
|`min`|1|0 &ge; n &lt; `max`|
|`max`|100,000|`min`&lt; n  &lt; 2<sup>64</sup>|
|`p`|20|1&ndash;100 (percentage)|
{.compact .params}

Used to access a fraction of data with intentional gaps between accessed records.

### int-range

Random ordered pairs `{n, n+size-1}` where `n` between `[min, max]` with uniform distribution
{.tagline}

|Param|Default|Valid Values (n)|
|-----|-------|----|
|`min`|1|int|
|`max`|100,000|int|
|`size`|100|&ge; 1|
{.compact .params}

Used for `BETWEEN @d AND @PREV`.

### int-range-seq

Sequential ordered pairs `{n, n+size-1}` from `begin` to `end`
{.tagline}

|Param|Default|Valid Values (n)|
|-----|-------|----|
|`begin`|1|int|
|`end`|100,000|int|
|`size`|100|&ge; 1|
{.compact .params}

Used to scan a table or index in order by a range of values: [1, 10], [11, 20].
When `end` is reached, restarts from `begin`.

### auto-inc

Monotonically increasing uint64 counter from `start` by `step` increments
{.tagline}

|Param|Default|Valid Value (n)|
|-----|-------|----|
|`start`|0|0 &le; n &lt; 2<sup>64</sup>|
|`step`|1|n &ge;1|
{.compact .params}

Every call adds `step` then returns the value.
By default starting at zero, returns 1, 2, 3, etc.
If `start = 10`, returns 11, 12, 13, etc.
If `start = 100` and `step = 5`, returns 105, 110, 115, etc.

## String

### str-fill-az

Fixed-length string filled with random characters a-z and A-Z
{.tagline}

|Param|Default|Valid Value (n)|
|-----|-------|----|
|`len`|100|n &ge; 1|
{.compact .params}

String length `len` is _characters_, not bytes.

## ID

### xid

Returns [rs/xid](https://github.com/rs/xid) values as strings.

## Column

The `column` generator is used for SQL modifiers [`save-insert-id`]({{< relref "syntax/trx-file#save-insert-id" >}}) and [`save-result`]({{< relref "syntax/trx-file#save-result" >}})
{.tagline}


|Param|Default|Valid Value|
|-----|-------|----|
|`quote-value`|yes|[string-bool]({{< relref "syntax/values#string-bool" >}})
{.compact .params}

The `quote-value` param determines if the value is quoted or not when used as output to a SQL statement:

* yes &rarr; `WHERE id = "%v"`
* no &rarr; `WHERE id = %v`

The underlying MySQL column type does not matter because the value is not cast to a data type; it's inputted and outputted as raw bytes.

The default [data scope]({{< relref "data/scope" >}}) for column data is _trx_, not statement.
This can be changed with an explicit scope configuration.
Iter data scope might be useful, but statement (or value) scope will probably not work since the purpose is to resue the value in another statment.
