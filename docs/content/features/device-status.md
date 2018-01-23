---
title: Device-status
menu:
    main:
        parent: features
        weight: 1
---


## Device-status

LoRa Server supports periodically requesting the device-status, using the
`DevStatusReq` mac-command specified by the LoRaWAN protocol.
A device will respond to this request by sending its battery status
(if available) and its demodulation signal-to-noise ratio in dB
for the last successfully received request.

When the device-status is available, it will be exposed to the application-server
on each received uplink payload.

### device-status battery

* `0` - The end-device is connected to an external power source
* `1..254` - The battery level, 1 being at minimum and 254 being at maximum
* `255` - The end-device was not able to measure the battery level

### device-status margin

The margin (Margin) is the demodulation signal-to-noise ratio in dB rounded
to the nearest integer value for the last successfully received DevStatusReq command.
