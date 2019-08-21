---
title: Prometheus
menu:
  main:
    parent: metrics
    weight: 1
description: Read metrics from the Prometheus metrics endpoint.
---

# Prometheus metrics

LoRa Server provides a [Prometheus](https://prometheus.io/) metrics endpoint
for monitoring the performance of the LoRa Server service. Please refer to
the [Prometheus](https://prometheus.io/) website for more information on
setting up and using Prometheus.

## Configuration

Please refer to the [Configuration documentation]({{<ref "install/config.md">}}).

## Metrics

### Go runtime metrics

These metrics are prefixed with `go_` and provide general information about
the process like:

* Garbage-collector statistics
* Memory usage
* Go go-routines

### gRPC API metrics

These metrics are prefixed with `grpc_` and provide metrics about the gRPC
API. e.g.:

* The number of times each API was called
* The duration of each API call (if enabled in the [Configuration]({{<ref "install/config.md">}}))

### Gateway backends

#### Azure IoT Hub

These metrics are prefixed with `backend_azure_iot_hub_` and provide:

* The number of received events by the Azure IoT Hub backend
* The number of published commands by the Azure IoT Hub backend


#### GCP Pub/Sub

These metrics are prefixed with `backend_gcp_pub_sub_` and provide:

* The number of received events by the GCP Pub/Sub backend
* The number of published commands to by the GCP Pub/Sub backend

#### MQTT


These metrics are prefixed with `backend_mqtt_` and provide:

* The number of received events by the MQTT backend
* The number of published commands by the MQTT backend
* The number of times the MQTT backend connected to the MQTT broker
* The number of times the MQTT backend disconnected from the MQTT broker

