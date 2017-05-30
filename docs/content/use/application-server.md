---
title: Application server
menu:
    main:
        parent: use
        weight: 1
---

## Application-server

The application-server is responsible for the node "inventory" part of a
LoRaWAN infrastructure, handling of received application payloads and
the downlink application payload queue. This component has been implemented by
[LoRa App Server](/lora-app-server/).

To implement your own application-server (and thus replace LoRa App Server),
your service needs to implement the [api/as/as.proto](https://github.com/brocaar/loraserver/tree/master/api/as/as.proto)
API. See also the [api]({{< ref "integrate/api.md" >}}) documentation for more
information.
