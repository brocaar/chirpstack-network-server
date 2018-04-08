---
title: Gateway-profile
menu:
    main:
        parent: features
        weight: 1
---

# Gateway-profile

The gateway-profile is an (optional) profile which can be assigned to a gateway
for the configuration of its channel-plan. This feature must also be configured
in the [LoRa Gateway Bridge configuration](/lora-gateway-bridge/install/config/).

## Fields

### Channels

The `channels` field defines the uplink channel-numbers that must be configured
for the gateways using this gateway-profile. These map to the channel-numbers
defined by the [LoRaWAN Regional Parameters](https://www.lora-alliance.org/lorawan-for-developers)
specification.

### Extra channels

The `extraChannels` field defines a list of extra channel that must be
configured for the gateway using this gateway-profile. Note that this is not
supported by every LoRaWAN band. Please consult the [LoRaWAN Regional Parameters](https://www.lora-alliance.org/lorawan-for-developers)
specification for more information.

## Hardware limitations

This feature is limited to 8-channel gateways (currently) and assumes that
channels can be distributed over two radios. When defining a channel-plan,
please keep in mind that the channels fit within the bandwidth of two radios.

The bandwidth of each radio depends on the bandwidth of the assigned channels:

* 500kHz channel = 1.1MHz radio bandwidth
* 250kHz channel = 1Mhz radio bandwidth
* 125kHz channel = 0.925MHz radio bandwidth
