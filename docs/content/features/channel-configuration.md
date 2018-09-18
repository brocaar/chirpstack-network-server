---
title: Channel (re)configuration
menu:
    main:
        parent: features
        weight: 2
description: Devices are automatically (re)configured to use the current configured channel-plan.
---

# Channel (re)configuration

LoRa Server supports the (re)configuration of the channels used by the device
for uplink transmissions. This feature can be used to configure additional
uplink channels (e.g. for the EU ISM band), or to enable only a sub-set of
the uplink channels specified by the [LoRaWAN Regional Parameters](https://www.lora-alliance.org/lorawan-for-developers).
See also [channel configuration]({{<ref "/install/config.md">}}).

## Additional channels

For certain regions, LoRa Server supports configuring additional channels
Please consult the [LoRaWAN Regional Parameters](https://www.lora-alliance.org/lorawan-for-developers)
to find out for which regions this applies.

When extra channels are configured, LoRa Server will configure the first 5
channels by using the OTAA join-accept `CFList` field. Additional channels
will be configured using the `NewChannelReq` mac-command. This mac-command
will also be used to push new or updated channel configuration to already
activated devices and to (re)configure the min/max data-rate range for these
extra channels.

## Enable sub-band

LoRa Server will by default assume that all available uplink channels specified
by the LoRaWAN Regional Parameters are active after an over-the-air activation.
As certain regions have more uplink channels than supported by most gateways,
it is possible to configure LoRa Server so that it will only enable a sub-set
of the available uplink-channels. Using the `LinkADRReq` mac-command it will
then update the channelmask of the device.

**Note:** after changing this setting, LoRa Server will push these changes at
the first opportunity to the already activated devices.
