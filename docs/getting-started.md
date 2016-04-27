# Getting started

## Requirements

Before you install the LoRa Server, make sure you've installed the requirements below:

#### MQTT server

LoRa Server makes use of MQTT for communication with the gateways and applications.
[Mosquitto](http://mosquitto.org/) is a popular open-source MQTT server.
Make sure you install a recent version of Mosquitto (the Mosquitto project provides
repositories for various Linux distributions).

#### PostgreSQL server

LoRa Server stores all persistent data into a [PostgreSQL](http://www.postgresql.org/) database. 

#### Redis

LoRa Server stores all session-related and non-persistent data into a [Redis](http://redis.io/) database.

#### LoRa Semtech Bridge

Most LoRa gateways use a [protocol](https://github.com/Lora-net/packet_forwarder/blob/master/PROTOCOL.TXT) defined by Semtech (UDP). To publish these packets over MQTT, you need to install [LoRa Semtech Bridge](https://github.com/brocaar/lora-semtech-bridge).

## Install LoRa Server

* Download and unpack a pre-compiled binary from the [releases](https://github.com/brocaar/loraserver/releases) page. Alternatively, build the code from source (not covered).

* For a full list of arguments that you can pass to the ``loraserver`` binary,
  run it with the ``--help`` flag. Note that all arguments can be passed as
  environment variables as well. When running ``loraserver`` with the
  ``--db-automigrate`` flag, it will automatically create (or update) the
  database schema (see the migrations folder in this repository).
  See [Configuration](configuration.md) for details on each config option.

* After you've started ``loraserver`` successfully, point your browser to [http://localhost:8000/](http://localhost:8000/).

* You should now be able to add applications and nodes and either activate your node by personalization (ABP) or over-the-air (OTAA).
