---
title: LoRa Server
menu:
    main:
        parent: overview
        weight: 1
---

# LoRa Server

LoRa Server is an open-source LoRaWAN network-server, part of the
[LoRa Server](https://docs.loraserver.io/) project. 
The responsibility of the network-server component is the de-duplication
and handling of received uplink frames received by the gateway(s), handling
of the LoRaWAN mac-layer and scheduling of downlink data transmissions.

## Features

The table below gives an overview of the features that LoRa Server offers.
See the **Features** in the left menu for a more detailed overview of each
feature.

|     |     |
| --- | --- |
| **LoRaWAN** | 1.0.x, 1.1.x |
| **Device classes** | Class-A (unicast), Class-B (unicast & multicast) & Class-C (unicast & multicast) |
| **Message types** | (Un)confirmed uplink and downlink, Proprietary uplink and downlink |
| **Device activation** | Over-the-air (OTAA) and activation by personalization              |
| **Adaptive data-rate** | Supported for all regions |
| **Regions supported** | AS 923<br />AU 915-928<br />CN 470-510<br />CN 779-787<br />EU 433<br />EU 863-870<br />IN 865-867<br />KR 920-923<br />US 902-928<br />RU 864-870 |
| **Frame-counter validation** | Strict (default)<br />Skip frame-counter mode (for debugging **only**) |
| **Statistics** | Per gateway received / transmitted (configurable aggregation levels) |
| **Mac-layer handling** | Channel (re)configuration<br />Adaptive data-rate<br />Device-status<br />Link check (initiated by the device)<br />Ping-slot channel configuration<br />Device time<br />RX parameter (re)configuration<br />Rejoin configuration |
| **Integration** | gRPC based API to an external application-server and external network-controller (optional) |
