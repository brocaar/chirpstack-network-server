---
title: Device-profile
menu:
    main:
        parent: features
        weight: 2
toc: false
description: Defines the device capabilities and boot parameters needed by LoRa Server.
---

# Device-profile

The device-profile defines the device capabilities and boot parameters that
are needed by LoRa Server to "connect" with a device.

## Fields / options

The following fields are described by the
[LoRaWAN Backend Interfaces specification](https://www.lora-alliance.org/lorawan-for-developers).
Fields marked with an **X** are implemented by LoRa Server.

- [X] **SupportsClassB** End-Device supports Class B
- [X] **ClassBTimeout** Maximum delay for the End-Device to answer a MAC request or a confirmed DL frame (mandatory if class B mode supported)
- [X] **PingSlotPeriod** Mandatory if class B mode supported
- [X] **PingSlotDR** Mandatory if class B mode supported
- [X] **PingSlotFreq** Mandatory if class B mode supported
- [X] **SupportsClassC** End-Device supports Class C
- [X] **ClassCTimeout** Maximum delay for the End-Device to answer a MAC request or a confirmed DL frame (mandatory if class C mode supported)
- [X] **MACVersion** Version of the LoRaWAN supported by the End-Device
- [X] **RegParamsRevision** Revision of the Regional Parameters document supported by the End-Device
- [X] **SupportsJoin** End-Device supports Join (OTAA) or not (ABP)
- [X] **RXDelay1** Class A RX1 delay (mandatory for ABP)
- [X] **RXDROffset1** RX1 data rate offset (mandatory for ABP)
- [X] **RXDataRate2** RX2 data rate (mandatory for ABP)
- [X] **RXFreq2** RX2 channel frequency (mandatory for ABP)
- [X] **FactoryPresetFreqs** List of factory-preset frequencies (mandatory for ABP)
- [X] **MaxEIRP** Maximum EIRP supported by the End-Device
- [ ] **MaxDutyCycle** Maximum duty cycle supported by the End-Device
- [X] **RFRegion** RF region name (automatically set by LoRa Server)
- [ ] **Supports32bitFCnt** End-Device uses 32bit FCnt (mandatory for LoRaWAN 1.0 End-Device) (always set to `true`)
