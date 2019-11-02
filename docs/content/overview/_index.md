---
title: ChirpStack Network Server
menu:
    main:
        parent: overview
        weight: 1
listPages: false
---

# ChirpStack Network Server

ChirpStack Network Server is an open-source LoRaWAN<sup>&reg;</sup> Network Server
implementation. This component is part of the ChirpStack stack.

The responsibility of the Network Server component is the de-duplication of
received LoRaWAN frames by the LoRa<sup>&reg;</sup> gateways and for the
collected frames handle the:

* Authentication
* LoRaWAN mac-layer (and mac-commands)
* Communication with the [ChirpStack Application Server](/application-server/)
* Scheduling of downlink frames

