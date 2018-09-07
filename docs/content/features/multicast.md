---
title: Multicast
menu:
    main:
        parent: features
        weight: 1
toc: false
---

# Multicast

LoRa Server has support for creating multicast-groups to which devices can be
assigned. When enqueueing a downlink payload for a multicast-group, LoRa Server
will analyze which gateways must be used for broadcasting to cover the complete
multicast-group. This means that potentially, a single multicast downlink
payload will be emitted multiple times. To avoid colissions, LoRa Server will
put a delay between multiple emissions.

Multicast can be used for the following device-classes:

* Class-B
* Class-C

The configuration of the multicast-groups at the device side happens out-of-band.
This means that Assigning a device to a device-group does not configure the
device itself to be part of the multicast-group.
