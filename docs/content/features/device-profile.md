---
title: Device-profile
menu:
    main:
        parent: features
        weight: 1
---

## Device-profile

The service-profile can be seen as the “contract” between an user and
the network. It describes the features that are enabled for the user(s)
of the service-profile and the rate of messages that can be sent over
the network.

### Fields / options

The following fields are described by the
[LoRaWAN Backend Interfaces specification](https://www.lora-alliance.org/lorawan-for-developers).
Fields marked with an **X** are implemented by LoRa Server.

- [ ] **SupportsClassB** End-Device supports Class B
- [ ] **ClassBTimeout** Maximum delay for the End-Device to answer a MAC request or a confirmed DL frame (mandatory if class B mode supported)
- [ ] **PingSlotPeriod** Mandatory if class B mode supported
- [ ] **PingSlotDR** Mandatory if class B mode supported
- [ ] **PingSlotFreq** Mandatory if class B mode supported
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
