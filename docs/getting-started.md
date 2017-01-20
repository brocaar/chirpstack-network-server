# Getting started

A complete LoRa Server setup, requires the setup of the following components:


* [LoRa Gateway Bridge](https://docs.loraserver.io/lora-gateway-bridge/)
* [LoRa Server](https://docs.loraserver.io/loraserver/)
* [LoRa App Server](https://docs.loraserver.io/lora-app-server/)


This getting started document describes the steps needed to setup LoRa Server
using the provided Debian package repository. Please note that LoRa Server
is not limited to Debian / Ubuntu only! General purpose binaries
can be downloaded from the 
[releases](https://github.com/brocaar/loraserver/releases) page.

!!! info
	An alternative way to setup all the components is by using the
	[loraserver-setup](https://github.com/brocaar/loraserver-setup) Ansible
	playbook. It automates the steps below and can also be used in combination
	with [Vagrant](https://www.vagrantup.com/).

!!! warning
    This getting started guide does not cover setting up firewall rules! After
    setting up LoRa Server and its requirements, don't forget to configure
    your firewall rules.

## Setting up LoRa Server

These steps have been tested with:

* Debian Jessie
* Ubuntu Trusty (14.04)
* Ubuntu Xenial (16.06)

### LoRa Server Debian repository

The LoRa Server project provides pre-compiled binaries packaged as Debian (.deb)
packages. In order to activate this repository, execute the following
commands:

```bash
source /etc/lsb-release
sudo apt-key adv --keyserver keyserver.ubuntu.com --recv-keys 1CE2AFD36DBCCA00
sudo echo "deb https://repos.loraserver.io/${DISTRIB_ID,,} ${DISTRIB_CODENAME} testing" | sudo tee /etc/apt/sources.list.d/loraserver.list
sudo apt-get update
```

### MQTT broker

LoRa Server makes use of MQTT for communication with the gateways 
[Mosquitto](http://mosquitto.org/) is a popular open-source MQTT
server. Make sure you install a **recent** version of Mosquitto.

For Ubuntu Trusty (14.04), execute the following command in order to add the
Mosquitto Apt repository:

```bash
sudo apt-add-repository ppa:mosquitto-dev/mosquitto-ppa
sudo apt-get update
```

In order to install Mosquitto, execute the following command:

```bash
sudo apt-get install mosquitto
```

### Redis

LoRa Server stores all session-related and non-persistent data into a
[Redis](http://redis.io/) datastore. Note that at least Redis 2.6.0 is required.
To Install Redis:

```bash
sudo apt-get install redis-server
```

### Install LoRa Server

In order to install LoRa Server, execute the following command:

```bash
sudo apt-get install loraserver
```

After installation, modify the configuration file which is located at
`/etc/default/loraserver`.

### Starting LoRa Server

How you need to (re)start and stop LoRa Server depends on if your
distribution uses init.d or systemd.

#### init.d

```bash
sudo /etc/init.d/loraserver [start|stop|restart|status]
```

#### systemd

```bash
sudo systemctl [start|stop|restart|status] loraserver
```

### LoRa Server log output

Now you've setup LoRa Server, it is a good time to verify that LoRa Server
is actually up-and-running. This can be done by looking at the LoRa Server
log output.

Like the previous step, which command you need to use for viewing the
log output depends on if your distribution uses init.d or systemd.

#### init.d

All logs are written to `/var/log/loraserver/loraserver.log`.
To view and follow this logfile:

```bash
tail -f /var/log/loraserver/loraserver.log
```

#### systemd

```bash
journalctl -u loraserver -f -n 50
```


Example output:

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
