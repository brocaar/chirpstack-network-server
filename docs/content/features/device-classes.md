---
title: Device-classes
menu:
    main:
        parent: features
        weight: 1
---

## Device-classes

### Class-A


#### Uplink

LoRa Server supports Class-A devices. Received frames are de-duplicated
(in case it has been received by multiple gateways), after which the mac-layer
is handled by LoRa Server and the encrypted application-playload is forwarded
to the [application server](https://docs.loraserver.io/lora-app-server).

#### Downlink

LoRa Server persists a downlink device-queue for to which the application-server
can enqueue downlink payloads. Once a receive window occurs, LoRa Server
will transmit the first downlink payload to the device.

##### Confirmed data

LoRa Server sends an acknowledgement to the application-server as soon one
is received from the device. When the next uplink transmission does not contain
an acknowledgement, a nACK is sent to the application-server.

**Note:** After a device (re)activation the device-queue is flushed.

### Class-B

Supported soon.

### Class-C

#### Downlink

LoRa Server supports Class-C devices and uses the same Class-A
downlink device-queue for Class-C downlink transmissions. The application-server
can enqueue one or multiple downlink payloads and LoRa Server will transmit
these (semi) immediately to the device, making sure no overlap exists in case
of multiple Class-C transmissions.

##### Confirmed data

LoRa Server sends an acknowledgement to the application-server as soon one
is received from the device. Until the frame has timed out, LoRa Server will
wait with the transmission of the next downlink Class-C payload.

**Note:** The timeout of a confirmed Class-C downlink can be configured through
the device-profile.
