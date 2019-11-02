---
title: Device Classes
menu:
    main:
        parent: features
        weight: 2
description: All device-classes (A/B/C) specified by the LoRaWAN specification are supported.
---

# Device-classes

## Class-A

### Uplink

ChirpStack Network Server supports LoRaWAN<sup>&reg;</sup> Class-A devices. In Class-A a device is always in sleep
mode, unless it has something to transmit. Only after an uplink transmission
by the device, ChirpStack Network Server is able to schedule a downlink transmission.

Received frames are de-duplicated (in case it has been received by multiple
gateways), after which the mac-layer is handled by ChirpStack Network Server and the
encrypted application-playload is forwarded to
the [Application Server](/application-server/).

### Downlink

ChirpStack Network Server persists a downlink device-queue for to which the application-server
can enqueue downlink payloads. Once a receive window occurs, ChirpStack Network Server
will transmit the first downlink payload to the device.

#### Confirmed data

ChirpStack Network Server sends an acknowledgement to the application-server as soon one
is received from the device. When the next uplink transmission does not contain
an acknowledgement, a nACK is sent to the application-server.

**Note:** After a device (re)activation the device-queue is flushed.

## Class-B

ChirpStack Network Server supports Class-B devices. A Class-B device synchronizes its
internal clock using Class-B beacons emitted by the gateway, this process
is also called a "beacon lock". Once in the state of a beacon lock, the
device negotioates its ping-interval. ChirpStack Network Server is then able to schedule
downlink transmissions on each occuring ping-interval. 

### Downlink

ChirpStack Network Server persists all downlink payloads in its device-queue. When the device
has acquired a beacon lock, it will schedule the payload for the next free ping-slot 
in the queue. When adding payloads to the queue when a beacon lock has not yet
been acquired, ChirpStack Network Server will update all device-queue to be scheduled
on the next free ping-slot once the device has acquired the beacon lock.

#### Confirmed data

ChirpStack Network Server sends an acknowledgement to the application-server as soon one
is received from the device. Until the frame has timed out, ChirpStack Network Server will
wait with the transmission of the next downlink Class-B payload.

**Note:** The timeout of a confirmed Class-B downlink can be configured through
the device-profile. This should be set to a value less than the interval between
two ping-slots.

### Requirements

#### Device

The device must be able to operate in Class-B mode. This feature has been
tested against the `develop` branch of the Semtech [LoRaMac-node](https://github.com/lora-net/LoRaMac-node/tree/develop)
source.

#### Gateway

The gateway must have a GNSS based time-source and must use at least
the [Semtech UDP Packet Forwarder](https://github.com/lora-net/packet_forwarder)
version 4.0.1 or higher. It also requires [ChirpStack Gateway Bridge](/gateway-bridge/).
2.2.0 or higher.

## Class-C

### Downlink

ChirpStack Network Server supports Class-C devices and uses the same Class-A
downlink device-queue for Class-C downlink transmissions. The application-server
can enqueue one or multiple downlink payloads and ChirpStack Network Server will transmit
these (semi) immediately to the device, making sure no overlap exists in case
of multiple Class-C transmissions.

#### Confirmed data

ChirpStack Network Server sends an acknowledgement to the application-server as soon one
is received from the device. Until the frame has timed out, ChirpStack Network Server will
wait with the transmission of the next downlink Class-C payload.

**Note:** The timeout of a confirmed Class-C downlink can be configured through
the device-profile.
