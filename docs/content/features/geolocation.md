---
title: Geolocation
menu:
    main:
        parent: features
        weight: 2
description: Decrypts the fine-timestamp of geolocation capable LoRa gateways and resolves the device location using a Geolocation Server.
---

# Geolocation

LoRa Server supports geolocation by using an external geolocation-server service.
You can either use the geolocation-server provided by the LoRa Server project,
or implement your own (matching the expected [api]({{<ref "/integrate/api.md" >}})).

## Requirements

For using geolocation, you need to use gateways that are capable of providing
a "fine-timestamp". Some of these gateways encrypt this fine-timestamp.

## Decrypt fine-timestamp

When configuring the per gateway and board specific decryption key, LoRa Server
will decrypt the fine-timestamp, before forwarding it to the geolocation-server.
For getting this fine-timestamp decryption key, please contact your gateway vendor
or Semtech.

## Device location

When LoRa Server (using the geolocation-server) is able to resolve the location
of the device, it will forward this to the application-server.
