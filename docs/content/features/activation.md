---
title: Activation
menu:
    main:
        parent: features
        weight: 1
---

# Device activation

## Over The Air Activation (OTAA)

In case of OTAA, LoRa Server will call the join-server configured
by the [`--js-server`]({{<ref "install/config.md">}}) flag which will
handle the authentication of the device and the generation of the session-keys.
The join-server API is specified by the [LoRaWAN Backend Interfaces](https://www.lora-alliance.org/lorawan-for-developers)
specification. By default [LoRa App Server](https://docs.loraserver.io/lora-app-server/)
fulfils this role.

## Activation By Personalization (ABP)

In case of ABP, LoRa Server has support for pre-activating devices through its
[API]({{<ref "integrate/api.md">}}). Once activated, LoRa Server will handle the
device in exactly the same way as an OTAA activated device.
