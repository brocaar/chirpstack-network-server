---
title: Routing-profile
menu:
    main:
        parent: features
        weight: 2
description: Defines to which application-server device-data must be routed.
---

# Routing-profile

By associating a routing-profile to a device, LoRa Server is able to forward
device data to the correct application-server. This allows to let LoRa Server
connect to one or multiple application-servers.

A routing-profile is created by using the network-server [api]({{<ref "/integrate/api.md">}}).
It is possible to associate a CA certificate and a client TLS certificate and key
with the routing-profile for authentication. This depends on the
[application-server configuration](https://docs.loraserver.io/lora-app-server/install/config/).
