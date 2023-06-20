---
weight: 2
title: "Client/Server"
---

## Client

Specify [`--server ADDR`]({{< relref "command-line/usage#--server" >}}) to run Finch as a client connected to the server at `ADDR`.

A client ignores other [command line options]({{< relref "command-line/usage#command-line-options" >}}) and automatically receives stage and trx files from the server.

The client runs only once.

## Server

Finch defaults to being a server listening on [`--bind`]({{< relref "command-line/usage#--bind" >}}).

The recommended client-server startup sequence is server then clients.

The server is counted as one compute instance called "local".
Set [`stage.compute.disable-local`]({{< relref "syntax/stage-file#disable-local" >}}) to disable.
