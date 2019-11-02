---
title: Activation
menu:
    main:
        parent: features
        weight: 2
description: Both Activation By Personalization (ABP) and Over The Air Activation (OTAA) are supported.
---

# Device activation

## Over The Air Activation (OTAA)

In case of a LoRaWAN<sup>&reg;</sup> OTAA activation request, ChirpStack Network Server will call the Join Server configured
in the [Configuration]({{<ref "/install/config.md">}}) file which will
handle the authentication of the device and the generation of the session-keys.
The Join Server API is specified by the [LoRaWAN Backend Interfaces](https://www.lora-alliance.org/lorawan-for-developers)
specification. By default [ChirpStack Application Server](/application-server/)
fulfils this role.

## Activation By Personalization (ABP)

In case of a LoRaWAN ABP devices, ChirpStack Network Server has support for pre-activating devices through its
[API]({{<ref "/integrate/api.md">}}). Once activated, ChirpStack Network Server will handle the
device in exactly the same way as an OTAA activated device.
