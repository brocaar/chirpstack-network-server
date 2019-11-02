---
title: Device Status
menu:
    main:
        parent: features
        weight: 2
description: Periodic device battery-status and link-margin request and reporting.
---


# Device Status

ChirpStack Network server supports periodically requesting the Device Status, using the
`DevStatusReq` mac-command specified by the LoRaWAN<sup>&reg;</sup> protocol.
A device will respond to this request by sending its battery status
(if available) and its demodulation signal-to-noise ratio in dB
for the last successfully received request.

When the Device Status is available, it will be exposed to the application-server
on each received uplink payload.

## Device Status battery

* `0` - The end-device is connected to an external power source
* `1..254` - The battery level, 1 being at minimum and 254 being at maximum
* `255` - The end-device was not able to measure the battery level

## Device Status margin

The margin (Margin) is the demodulation signal-to-noise ratio in dB rounded
to the nearest integer value for the last successfully received DevStatusReq command.
