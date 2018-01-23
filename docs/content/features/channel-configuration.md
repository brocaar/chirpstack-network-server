---
title: Channel (re)configuration
menu:
    main:
        parent: features
        weight: 1
---

## Channel (re)configuration

LoRa Server supports the (re)configuration of the channels used by the device
for uplink transmissions. This feature can be used to configure additional
uplink channels (e.g. for the EU ISM band), or to enable only a sub-set of
the uplink channels specified by the [LoRaWAN Regional Parameters](https://www.lora-alliance.org/lorawan-for-developers).
See also [channel configuration]({{<ref "install/config.md">}}).

### Additional channels

For certain regions, LoRa Server supports adding 5 additional channels. The device
will be made aware of these channels during an over-the-air activation.

### Enable sub-band

LoRa Server will by default assume that all available uplink channels specified
by the LoRaWAN Regional Parameters are active after an over-the-air activation.
As certain regions have more uplink channels than supported by most gateways,
it is possible to configure LoRa Server so that it will only enable a sub-set
of the available uplink-channels.
