# Getting started

## Requirements

Before you install the LoRa Server, make sure you've installed the following requirements:

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

Most LoRa gateways are using a [UDP protocol](https://github.com/Lora-net/packet_forwarder/blob/master/PROTOCOL.TXT)
defined by Semtech (UDP). To publish these packets over MQTT, you need to install
[LoRa Semtech Bridge](https://github.com/brocaar/lora-semtech-bridge).

## Install LoRa Server

#### Download

Download and unpack a pre-compiled binary from the
[releases](https://github.com/brocaar/loraserver/releases) page. Alternatively,
build the code from source.

#### Configuration

All configuration is done by either environment variables or command-line
arguments. Arguments and environment variables can be mixed. 

Run ``./loraserver --help`` for a list of available arguments.
See [Configuration](configuration.md) for details on each config option.
It is a good idea to start LoRa Server with ``--db-automigrate`` so that
database tables will be created automatically.

#### Starting LoRa Server

When you've created a PostgreSQL database, it is time to start LoRa Server.
This might look like:

```base
./loraserver \
	--db-automigrate \  # apply database migrations on start
	--net-id 010203 \   # LoRaWAN NetID, see specifications for details
	--postgres-dsn "postgres://user:password@localhost/loraserver?sslmode=disable"  # PostgreSQL credentials, hostname and database
```

#### Create application and node

Now that your LoRa Server instance is running, it is time to create
your first Application and Node. Point your browser to 
[http://localhost:8000/](http://localhost:8000/) for the web-interface.

#### Provision node

Now you have setup the AppEUI, DevEUI and AppKey in the web-interface,
you're able to provision your node. See [activating nodes](activating-nodes.md)
for more details.
