---
title: FAQ
menu:
    main:
        parent: community
        weight: 4
---

## Frequently asked questions

##### Does LoRa Server implement a queue for downlinks payloads?

No, this component is now part of [LoRa App Server](/lora-app-server/).

##### Does LoRa Server support ADR?

Yes, LoRa Server has experimental support for ADR.

##### Packets are not received / OTAA does not work

There are many things that can go wrong, and setting up a LoRaWAN
infrastructure can be frustrating when done the first time. First of all,
take a close look to the logs of LoRa Server and the [LoRa Gateway Bridge](/lora-gateway-bridge/)
as it might give you a clue what is is going wrong. When that doesn't help,
take a look at the [packet_forwarder](https://github.com/Lora-net/packet_forwarder)
logs as it might give more information why packets are being dropped by the
gateway. The [LoRa Gateway Bridge FAQ](/lora-gateway-bridge/community/faq/)
has more information about debugging the packet_forwarder.

##### transport: dial tcp 127.0.0.1:8001: getsockopt: connection refused

When you see the error `dial tcp 127.0.0.1:8001: getsockopt: connection refused`
in your logs, it means LoRa Server is unable to connect to the
application-server. See also [LoRa App Server](/lora-app-server/).
