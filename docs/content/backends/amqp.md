---
title: AMQP / RabbitMQ
menu:
  main:
    parent: backends
    weight: 3
description: Backend which uses AMQP / RabbitMQ (with MQTT plugin) for communication between the LoRa gateways and the ChirpStack Network Server.
---

# AMQP / RabbitMQ gateway backend

The AMQP / [RabbitMQ](https://www.rabbitmq.com) backend uses RabbitMQ together
with the [RabbitMQ MQTT plugin](https://www.rabbitmq.com/mqtt.html) for
communication with the LoRa<sup>&reg;</sup> gateways.

The RabbitMQ MQTT plugin is used to connect the [ChirpStack Gateway Bridge](/gateway-bridge/)
instances using MQTT. The RabbitMQ MQTT plugin will publish received events
to the `amq.topic` exchange using the routing key `gateway.ID.event.EVENT`.
This plugin will also create a queue per connection with a binding to
`gateway.ID.command.#`.

The ChirpStack Network Server will, in case one doesn't exist yet, create
a `gateway-events` queue with a binding to the `gateway.*.event.*` routing-key
and will consume received events from this queue.

Downlink gateway commands are published by the ChirpStack Network Server to
the `amq.topic` exchange to the `gateway.ID.command.COMMAND` topic. As the
RabbitMQ automatically creates a binding to `gateway.ID.command.#`, these
commands are routed to the gateway with the corresponding `ID`.

**Note:** the above routing-keys are assuming default configuration parameters.


## Architecture

[![architecture](/network-server/img/graphs/backends/amqp_rabbitmq.png)](/network-server/img/graphs/backends/amqp_rabbitmq.png)
