---
title: LoRa Server
menu:
    main:
        parent: overview
        weight: 1
---

## LoRa Server

LoRa Server is an open-source LoRaWAN network-server. It is responsible for
handling (and de-duplication) of uplink data received by the gateway(s) and
the scheduling of downlink data transmissions.

### Features

#### Device classes

##### Class A

LoRa Server has full support for Class-A devices. Received payloads are
de-duplicated (in case they are picked up by multiple gateways), after which
they are forwarded to the [application server](https://docs.loraserver.io/lora-app-server/).
When a receive-window occurs, LoRa Server will poll the [application server](https://docs.loraserver.io/lora-app-server/)
for downlink data. By polling instead of letting the application server push the
data, the application server is able to respect the maximum payload size for the
data-rate used for the downlink transmission.

##### Class B

Todo.

##### Class C

LoRa Server has full support for Class-C devices. It will remember the last
rx parameters (of each gateway that received the uplink), so that the
nearest gateway can be used for the Class-C downlink. A downlink can be scheduled
by using the `NetworkServer.PushDataDown` API method.

#### Confirmed data up / down

Both uplink and downlink confirmed data is handled by LoRa Server. In case of
a downlink (confirmed) payload, LoRa Server will keep the payload in its queue
until it has been acknowledged by the node.

#### Node activation

LoRa Server has support for both ABP (activation by personalization) and OTAA
(over the air activation). In case of ABP, the [application server](https://docs.loraserver.io/lora-app-server/)
provisions LoRa Server with a node-session. In case of OTAA, LoRa Server will
call the [application server](https://docs.loraserver.io/lora-app-server/) with
the received join-request and in case of a positive response, it will transmit
the join-accept to the node.

#### Adaptive data-rate (experimental)

LoRa Server has support for adaptive data-rate (ADR). In order to activate ADR,
The node must have the ADR interval and installation margin configured. The
first one contains the number of frames after which to re-calculate the ideal
data-rate and TX power of the node, the latter one holds the installation margin
of the network (the default recommended value is 5dB). From the node-side it is
required that the ADR flag is set for each uplink transmission.

**Important:** ADR is only suitable for static devices, thus devices that do
not move! 

#### Gateway management and stats

Gateways can be created either automatically when LoRa Server receives
statistics from the gateways or by using the API. Gateway statistics will be
aggregated on the given intervals and are exposed through the 
[api](api.md) API. See also [gateway management](gateway-management.md).

#### Receive windows

Through OTAA and ABP, it is possible to configure which RX window to use for
downlink transmissions. This also includes the parameters like data-rate
(for RX2) and the delay to use.

#### Relax frame-counter

A problem with many ABP devices is that after a power-cycle, the frame-counter
of the device is reset. Since this reset is not known by LoRa Server it means
that all payloads with a frame-counter smaller or equal than the known counter
get rejected. In order to work around this issue it is possible to enable
the relax frame-counter mode. Important to know, this compromises security!

#### ISM bands

As different regions have have different regulations regarding the license-free
bands, you have to specify the ISM band to operate on when starting LoRa Server.
At this moment the following ISM bands have been implemented:

- AS 923
- AU 915-928
- CN 470-510
- CN 779-787
- EU 433
- EU 863-870
- KR 920-923
- RU 864-869
- US 902-928
