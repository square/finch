---
---

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
