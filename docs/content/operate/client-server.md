---
weight: 2
title: "Client/Server"
---

## Client

Specify [`--client ADDR`]({{< relref "operate/command-line#--client" >}}) to run Finch as a client connected to the server at `ADDR`.

A client ignores other [command line options]({{< relref "operate/command-line#command-line-options" >}}) and automatically receives stage and trx files from the server.

The client runs only once.
This is largely due to https://bugs.mysql.com/bug.php?id=110941: MySQL doesn't properly terminate clients/connections in some cases, especially when the client aborts the connection, which is what the Go MySQL driver does on context cancellation.

## Server

Specify [`--server ADDR`]({{< relref "operate/command-line#--server" >}}) to run Finch as a server that listens on ADDR[:PORT} for remote compute instances.

The recommended client-server startup sequence is server then clients: start the server, then start the clients.

The server is counted as one compute instance called "local".
Set [`stage.compute.disable-local`]({{< relref "syntax/stage-file#disable-local" >}}) to disable.

{{< hint type=note >}}
A standalone instance of Finch is a pseduo-server that doesn't bind to an interface and runs only locally.
As a result, [`--debug`]({{< relref "operate/command-line#--debug" >}}) prints server info even when `--server` is not specififed.
{{< /hint >}}

## Protocol

The client-server protocol is initiated by clients over a standard HTTP port.
The server needs to allow incoming HTTP on that port (default 33075).

{{< mermaid class="text-center" >}}
sequenceDiagram
    autonumber

    activate client
    client->>server: GET /boot
    server-->>client: return stage files

    loop Every trx file
        client->>server: GET /file?trx=N
        server-->>client: return trx file N
    end
    
    client->>server: POST /boot
    server-->>client: ack
    
    client->>server: GET /run
    deactivate client
    Note over client: Client waits for server
    Note over server: Server waits for stage.compute.instances

    server-->>client: ack
    
    activate client
    Note left of client: Client runs stages
    loop While running
        client->>server: POST /stats
        server-->>client: ack
    end
    deactivate client
    
    Note left of client: Client done running
    client->>server: POST /run
    server-->>client: ack
{{< /mermaid >}}
