---
weight: 4
title: Values
---

## String-int

In Finch [stage]({{< relref "syntax/stage-file" >}}) and [\_all.yaml]({{< relref "syntax/all-file" >}}) files, most numerical values are YAML strings.

|String Value|Numerical Value|
|------------|---------------|
|""|0 or default value|
|"1"|1|
|"1,000"|1000|
|"1k"|1000|
|"1KB"|1000|
|"1KiB"|1024|
|"$params.foo"|5 when `params.foo = "5"`|

## Time Duration

Time durations (usually a configuration or parameter called `freq`) are [Go time duration](https://pkg.go.dev/time#ParseDuration) strings:

>Valid time units are "ns", "us" (or "µs"), "ms", "s", "m", "h".

## String-bool

These strings evaluate to boolean true:

* "true"
* "yes"
* "on"
* "aye"

All other strings, including empty strings, evaluate to boolean false.
