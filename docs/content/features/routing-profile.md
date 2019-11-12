---
title: Routing Profile
menu:
    main:
        parent: features
        weight: 2
description: Defines to which application-server device-data must be routed.
---

# Routing-profile

By associating a Routing Profile to a device, ChirpStack Network Server is able to forward
device data to the correct LoRaWAN<sup>&reg;</sup> Application Server, for example
[ChirpStack Application Server](/application-server/). 

A Routing Profile is created by using the Network Server [API]({{<ref "/integrate/api.md">}}).
It is possible to associate a CA certificate and a client TLS certificate and key
with the Routing Profile for authentication. This depends on the
[ChirpStack Application Server Configuration](/application-server/install/config/).
