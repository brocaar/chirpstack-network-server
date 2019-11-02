---
title: Device Time
menu:
    main:
        parent: features
        weight: 2
toc: false
description: Synchronize the internal clock of a device to the network’s clock.
---

# Device Time

ChirpStack Network Server supports the synchronization of the internal device clock with the
network using the `DeviceTimeReq` mac-command. This is useful for devices that
need to have an accurate time-source or devices implementing LoRaWAN<sup>&reg;</sup> Class-B.

When possible, ChirpStack Network Server uses the RX timestamp provided by the gateway which
results in the most accurate time. When this timestamp is not available (e.g. the
gateway is not time synchronized), it will use the current server time. Please
note that in this case the returned timestamp is less accurate as ChirpStack Network Server
is not aware of the latency between the gateway and the network-server.
