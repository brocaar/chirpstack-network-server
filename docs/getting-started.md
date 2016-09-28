# Getting started

This getting started document describes the steps needed to setup LoRa Server
and all its requirements on Ubuntu 16.04 LTS. When using an other Linux
distribution, you might need to adapt these steps.

!!! warning
    This getting started guide does not cover setting up firewall rules! After
    setting up LoRa Server and its requirements, don't forget to configure
    your firewall rules.

## MQTT broker

LoRa Server makes use of MQTT for communication with the gateways (thought the
[LoRa Gateway Bridge](http://docs.loraserver.io/lora-gateway-bridge/)).
[Mosquitto](http://mosquitto.org/) is a
popular open-source MQTT broker. Make sure you install a recent version of
Mosquitto (the Mosquitto project provides repositories for various Linux
distributions). Ubuntu 16.04 LTS already includes a recent version which can be
installed with:

```bash
sudo apt-get install mosquitto
```

## Redis

LoRa Server stores all session-related and non-persistent data into a
[Redis](http://redis.io/) datastore. Note that at least Redis 2.6.0 is required.
To Install Redis:

```bash
sudo apt-get install redis-server
```

## LoRa Gateway Bridge

As most LoRa gateways are using the [packet_forwarder](https://github.com/Lora-net/packet_forwarder)
which uses a [UDP protocol](https://github.com/Lora-net/packet_forwarder/blob/master/PROTOCOL.TXT)
for communication, you need to setup the [LoRa Gateway Bridge](http://docs.loraserver.io/lora-gateway-bridge/)
which abstracts this UDP protocol into JSON over MQTT. These installation steps
are documented in the [LoRa Gateway Bridge documentation](http://docs.loraserver.io/lora-gateway-bridge/).

## LoRa App Server

As LoRa Server itself is only aware about node-sessions and doesn't know
anything about the inventory, you need to run an application-server compatible
with the [ApplicationServer api](https://github.com/brocaar/loraserver/blob/master/api/as/as.proto).
Instructions to setup [LoRa App Server](https://github.com/brocaar/lora-app-server)
are documented in [LoRa App Server documentation](http://docs.loraserver.io/lora-app-server/).

## Install LoRa Server

Create a system user for loraserver:

```bash
sudo useradd -M -r -s /bin/false loraserver
```

Download and unpack a pre-compiled binary from the
[releases](https://github.com/brocaar/loraserver/releases) page:

```bash
# replace VERSION with the latest version or the version you want to install

# download
wget https://github.com/brocaar/loraserver/releases/download/VERSION/loraserver_VERSION_linux_amd64.tar.gz

# unpack
tar zxf loraserver_VERSION_linux_amd64.tar.gz

# move the binary to /opt/loraserver/bin
sudo mkdir -p /opt/loraserver/bin
sudo mv loraserver /opt/loraserver/bin
```

In order to start LoRa Server as a service, create the file
`/etc/systemd/system/loraserver.service` with as content:

```
[Unit]
Description=loraserver
After=mosquitto.service

[Service]
User=loraserver
Group=loraserver
ExecStart=/opt/loraserver/bin/loraserver
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

In order to configure LoRa Server, create a directory named
`/etc/systemd/system/loraserver.service.d`:

```bash
sudo mkdir /etc/systemd/system/loraserver.service.d
```

Inside this directory, put a file named `loraserver.conf`:

```
[Service]
Environment="NET_ID=010203"
Environment="BAND=EU_863_870"
Environment="BIND=127.0.0.1:8000"
Environment="REDIS_URL=redis://localhost:6379"
Environment="GW_MQTT_SERVER=tcp://localhost:1883"
Environment="AS_SERVER=127.0.0.1:8001"
```

## Starting LoRa Server

In order to (re)start and stop LoRa Server:

```bash
# start
sudo systemctl start loraserver

# restart
sudo systemctl restart loraserver

# stop
sudo systemctl stop loraserver
```

Verifiy that LoRa Server is up-and running by looking at its log-output:

```bash
journalctl -u loraserver -f -n 50
```

The log should be something like:

```
INFO[0000] starting LoRa Server                          band=EU_863_870 docs=https://docs.loraserver.io/ net_id=010203 version=0.12.0
INFO[0000] setup redis connection pool                   url=redis://localhost:6379
INFO[0000] backend/gateway: connecting to mqtt broker    server=tcp://localhost:1883
INFO[0000] connecting to application-server              ca-cert= server=127.0.0.1:8001 tls-cert= tls-key=
INFO[0000] backend/gateway: connected to mqtt server
INFO[0000] backend/gateway: subscribing to rx topic      topic=gateway/+/rx
INFO[0000] no network-controller configured
INFO[0000] starting api server                           bind=0.0.0.0:8000 ca-cert= tls-cert= tls-key=
```

When you get the following log-messages, it means that LoRa Server can't
connect to the application-server.

```
INFO[0000] grpc: addrConn.resetTransport failed to create client transport: connection error: desc = "transport: dial tcp 127.0.0.1:8001: getsockopt: connection refused"; Reconnecting to {"127.0.0.1:8001" <nil>}
INFO[0001] grpc: addrConn.resetTransport failed to create client transport: connection error: desc = "transport: dial tcp 127.0.0.1:8001: getsockopt: connection refused"; Reconnecting to {"127.0.0.1:8001" <nil>}
INFO[0002] grpc: addrConn.resetTransport failed to create client transport: connection error: desc = "transport: dial tcp 127.0.0.1:8001: getsockopt: connection refused"; Reconnecting to {"127.0.0.1:8001" <nil>}
```

## Configuration

In the example above, we've just touched a few configuration variables.
Run `loraserver --help` for an overview of all available variables. Note
that configuration variables can be passed as cli arguments and / or environment
variables (which we did in the above example).

See [Configuration](configuration.md) for details on each config option.
