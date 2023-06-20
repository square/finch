---
weight: 5
---

There are three types of parameters: user-defined, built-in, and environment variable.

{{< toc >}}

## User-defined

User-defined parameters are defined in [\_all.yaml](/syntax/all-file/#params), and [stage files](/syntax/stage-file/), and by [`--param`](/command-line/usage/#--param) on the command line.

```yaml
params:
  foo: "bar"
  rows: "100k"
```

* "$params.foo" &rarr; "bar"
* "$params.rows" &rarr; "100000"

The "$params." prefix is required.
It can be wrapped in curly braces: "${params.foo}".

## Built-in

|Param|Value|
|----|------|
|$sys.CPU_CORES|Number of CPU cores detected by Go|

Built-in parameters are used as shown in the table above (no "$params." prefix).

## Environment Variable

If a parameter isn't user-defined or built-in, Finch tries to fetch it as an environment variable.
For example, if `HOME=/home/finch`, then "$HOME" &rarr; "/home/finch".

## Inheritance 

{{< columns >}} <!-- begin columns block -->
_\_all.yaml_
```yaml
params:
  foo: "bar"
  rows: "100k"
```
<--->
_+ Stage File_
```yaml
stage:
  params:
    rows: "500"
    clients: 16
```
<--->
_= Final Params_
```
foo = bar
rows = 500
clients = 16
```
{{< /columns >}}

[`--param`](/command-line/usage/#--param) on the command line overrides all and sets the final value.

Reusing the example above, `--params rows=999` overrides both \_all.yaml and the stage file, setting final value rows = 999.

{{< hint type=tip >}}
Look for "param:" in the [`--debug`](/command-line/usage/#--debug) output to debug parameters processing.
{{< /hint >}}
