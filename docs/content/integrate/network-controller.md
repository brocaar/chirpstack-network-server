---
title: Network Controller
menu:
  main:
    parent: integrate
    weight: 3
description: Information about the Network Controller and how to implement one.
---

# Network Controller

ChirpStack Network Server makes it possible to integrate an external Network Controller
to interact with the ChirpStack Network Server API. This makes it possible to let an external
component schedule mac-commands for example.

For this to work, the external Network Controller must implement the
`NetworkController` gRPC service as specified in
[`api/nc/nc.proto`](https://github.com/brocaar/chirpstack-network-server/blob/master/api/nc/nc.proto).
Also ChirpStack Network Server must be configured so that it connects to this Network Controller
(see [Configuration]({{< ref "/install/config.md" >}})).

**Note:** for "regular" use, this component is not needed as most / all
mac-commands are already scheduled by ChirpStack Network Server, based on the ChirpStack Network Server
[Configuration]({{< ref "/install/config.md" >}}).
