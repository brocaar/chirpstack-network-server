---
title: MQTT
menu:
  main:
    parent: backends
    weight: 2
description: Backend which uses MQTT for communication between the LoRa gateways and the ChirpStack Network Server (default).
---

# MQTT gateway backend

The MQTT backend is the default backend to communicate with the LoRa<sup>&reg;</sup>
gateways. As round-robin message delivery is not possible with (all) MQTT brokers,
this backend implements its own de-duplication, to assure that a gateway event
is only handled once.

## Architecture

[![architecture](/network-server/img/graphs/backends/mqtt.png)](/network-server/img/graphs/backends/mqtt.png)

**Note:** In the graph above, the [ChirpStack Gateway Bridge](/gateway-bridge/) is
installed on the gateway. It is also possible to install the ChirpStack
Gateway Bridge in the cloud.
