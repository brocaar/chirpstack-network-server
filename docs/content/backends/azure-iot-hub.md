---
title: Azure IoT Hub
menu:
  main:
    parent: backends
    weight: 3
description: Backend which uses the Azure IoT Hub service for communication between the LoRa gateways and the ChirpStack Network Server.
---

# Azure IoT Hub gateway backend

The [Azure IoT Hub](https://azure.microsoft.com/en-us/services/iot-hub/) backend
uses the Azure IoT Hub and [Azure Service Bus](https://azure.microsoft.com/en-us/services/service-bus/)
services to communicate with the LoRa<sup>&reg;</sup> gateways. 

[ChirpStack Gateway Bridge](/gateway-bridge/) instances are connected using the
Azure IoT Hub [MQTT bridge](https://docs.microsoft.com/en-us/azure/iot-hub/iot-hub-mqtt-support)
and the Azure IoT Hub forwards received gateway events to an Azure Service-Bus Queue
which is consumed by the ChirpStack Network Server instance or instances.

Downlink gateway commands are published by the ChirpStack Network Server directly
to the Azure IoT Hub, which will forward the gateway commands over MQTT to the
ChirpStack Gateway Bridge.

## Architecture

[![architecture](/network-server/img/graphs/backends/azure_iot_hub.png)](/network-server/img/graphs/backends/azure_iot_hub.png)
