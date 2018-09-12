---
title: Service-profile
menu:
    main:
        parent: features
        weight: 1
toc: false
---

# Service-profile

The service-profile defines the features that are enabled for the associated
devices and the rate of messages that the associated devices can send over
the network.

## Fields / options

The following fields are described by the
[LoRaWAN Backend Interfaces specification](https://www.lora-alliance.org/lorawan-for-developers).
Fields marked with an **X** are implemented by LoRa Server.

- [ ] **ULRate** Token bucket filling rate, including ACKs (packet/h)
- [ ] **ULBucketSize** Token bucket burst size
- [ ] **ULRatePolicy** Drop or mark when exceeding ULRate
- [ ] **DLRate** Token bucket filling rate, including ACKs (packet/h)
- [ ] **DLBucketSize** Token bucket burst size
- [ ] **DLRatePolicy** Drop or mark when exceeding DLRate
- [X] **AddGWMetadata** GW metadata (RSSI, SNR, GW geoloc., etc.) are added to the packet sent to AS
- [X] **DevStatusReqFreq** Frequency to initiate an End-Device status request (request/day)
- [X] **ReportDevStatusBattery** Report End-Device battery level to AS
- [X] **ReportDevStatusmargin** Report End-Device margin to AS
- [X] **DRMin** Minimum allowed data rate. Used for ADR.
- [X] **DRmax** Maximum allowed data rate. Used for ADR.
- [ ] **ChannelMask** Channel mask. sNS does not have to obey (i.e., informative).
- [ ] **PRAllowed** Passive Roaming allowed
- [ ] **HRAllowed** Handover Roaming allowed
- [ ] **RAAllowed** Roaming Activation allowed
- [X] **NwkGeoLoc** Enable network geolocation service
- [ ] **TargetPER** Target Packet Error Rate
- [ ] **MinGWDiversity** Minimum number of receiving GWs (informative)
