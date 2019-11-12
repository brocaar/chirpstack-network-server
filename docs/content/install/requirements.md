---
title: Requirements
menu:
    main:
        parent: install
        weight: 1
description: Instruction on how to setup the ChirpStack Network Server requirements.
---

# Requirements

The mentioned Debian / Ubuntu instructions have been tested on:

* Ubuntu 18.04 (LTS)
* Debian 10 (Stretch)

## MQTT broker

ChirpStack Network Server makes use of MQTT for publishing and receiving application
payloads. [Mosquitto](http://mosquitto.org/) is a popular open-source MQTT
server, but any MQTT broker implementing MQTT 3.1.1 should work.
In case you install Mosquitto, make sure you install a **recent** version.

### Install

#### Debian / Ubuntu

In order to install Mosquitto, execute the following command:

{{<highlight bash>}}
sudo apt-get install mosquitto
{{< /highlight >}}

#### Other platforms

Please refer to the [Mosquitto download](https://mosquitto.org/download/) page
for information about how to setup Mosquitto for your platform.

## PostgreSQL database

ChirpStack Network Server persists the gateway data into a
[PostgreSQL](https://www.postgresql.org) database. Note that PostgreSQL 9.5+
is required.

### Install

#### Debian / Ubuntu

To install PostgreSQL:

{{<highlight bash>}}
sudo apt-get install postgresql
{{< /highlight >}}

#### Other platforms

Please refer to the [PostgreSQL download](https://www.postgresql.org/download/)
page for information how to setup PostgreSQL on your platform.

## Redis database

ChirpStack Network Server stores all non-persistent data into a
[Redis](http://redis.io/) datastore. Note that at least Redis 2.6.0
is required.

### Install

#### Debian / Ubuntu

To Install Redis:

{{<highlight bash>}}
sudo apt-get install redis-server
{{< /highlight >}}

#### Other platforms

Please refer to the [Redis](https://redis.io/) documentation for information
about how to setup Redis for your platform.
