---
weight: 2
title: "Client/Server"
---

## Client

Specify [`--server ADDR`](../usage/#--server) to run Finch as a client connected to the server at `ADDR`.

A client ignores other [command line options](../usage/#command-line-options) and automatically receives stage and trx files from the server.

The client runs only once.

## Server

Finch defaults to being a server listening on [`--bind`](../usage/#--bind).

The recommended client-server startup sequence is server then clients.

The server is counted as one compute instance called "local".
Set [`stage.compute.disable-local`](/syntax/stage-file/#disable-local) to disable.
