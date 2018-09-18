---
title: Network-controller
menu:
  main:
    parent: integrate
    weight: 3
description: Information about the Network Controller and how to implement one.
---

# Network-controller

LoRa Server makes it possible to integrate an external network-controller
to interact with the LoRa Server API. This makes it possible to let an external
component schedule mac-commands for example.

For this to work, the external network-controller must implement the
`NetworkController` gRPC service as specified in
[`api/nc/nc.proto`](https://github.com/brocaar/loraserver/blob/master/api/nc/nc.proto).
Also LoRa Server must be configured so that it connects to this network-controller
(see [configuration]({{< ref "/install/config.md" >}})).

**Note:** for "regular" use, this component is not needed as most / all
mac-commands are already scheduled by LoRa Server, based on the LoRa Server
[configuration]({{< ref "/install/config.md" >}}).
