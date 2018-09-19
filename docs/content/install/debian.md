---
title: Debian / Ubuntu
menu:
    main:
        parent: install
        weight: 2
description: Instructions how to install LoRa Server on a Debian or Ubuntu based Linux installation.
---

# Debian / Ubuntu installation

These steps have been tested on:

* Ubuntu 16.04 (LTS)
* Ubuntu 18.04 (LTS)
* Debian 9 (Stretch)

## Creating an user and database

LoRa Server needs its **own** database. To create a new database,
start the PostgreSQL prompt as the `postgres` user:

{{<highlight bash>}}
sudo -u postgres psql
{{< /highlight >}}

Within the the PostgreSQL prompt, enter the following queries:

{{<highlight sql>}}
-- create the loraserver_ns user with password 'dbpassword'
create role loraserver_ns with login password 'dbpassword';

-- create the loraserver_ns database
create database loraserver_ns with owner loraserver_ns;

-- exit the prompt
\q
{{< /highlight >}}

To verify if the user and database have been setup correctly, try to connect
to it:

{{<highlight bash>}}
psql -h localhost -U loraserver_ns -W loraserver_ns
{{< /highlight >}}


## LoRa Server Debian repository

The LoRa Server project provides pre-compiled binaries packaged as Debian (.deb)
packages. In order to activate this repository, execute the following
commands:

{{<highlight bash>}}
sudo apt-key adv --keyserver keyserver.ubuntu.com --recv-keys 1CE2AFD36DBCCA00

sudo echo "deb https://artifacts.loraserver.io/packages/2.x/deb stable main" | sudo tee /etc/apt/sources.list.d/loraserver.list
sudo apt-get update
{{< /highlight >}}

## Install LoRa Server

In order to install LoRa Server, execute the following command:

{{<highlight bash>}}
sudo apt-get install loraserver
{{< /highlight >}}

After installation, modify the configuration file which is located at
`/etc/loraserver/loraserver.toml`.

Settings you probably want to set / change:

* `postgresql.dsn`
* `postgresql.automigrate`
* `network_server.net_id`
* `network_server.band.name`
* `network_server.gateway.stats.timezone`

## Starting LoRa Server

How you need to (re)start and stop LoRa Server depends on if your
distribution uses init.d or systemd.

### init.d

{{<highlight bash>}}
sudo /etc/init.d/loraserver [start|stop|restart|status]
{{< /highlight >}}

### systemd

{{<highlight bash>}}
sudo systemctl [start|stop|restart|status] loraserver
{{< /highlight >}}

## LoRa Server log output

Now you've setup LoRa Server, it is a good time to verify that LoRa Server
is actually up-and-running. This can be done by looking at the LoRa Server
log output.

Like the previous step, which command you need to use for viewing the
log output depends on if your distribution uses init.d or systemd.

### init.d

All logs are written to `/var/log/loraserver/loraserver.log`.
To view and follow this logfile:

{{<highlight bash>}}
tail -f /var/log/loraserver/loraserver.log
{{< /highlight >}}

### systemd

{{<highlight bash>}}
journalctl -u loraserver -f -n 50
{{< /highlight >}}


Example output:

{{<highlight text>}}
INFO[0000] starting LoRa Server                          band=EU_863_870 docs=https://docs.loraserver.io/ net_id=010203 version=0.12.0
INFO[0000] setup redis connection pool                   url=redis://localhost:6379
INFO[0000] backend/gateway: connecting to mqtt broker    server=tcp://localhost:1883
INFO[0000] connecting to application-server              ca-cert= server=127.0.0.1:8001 tls-cert= tls-key=
INFO[0000] backend/gateway: connected to mqtt server
INFO[0000] backend/gateway: subscribing to rx topic      topic=gateway/+/rx
INFO[0000] no network-controller configured
INFO[0000] starting api server                           bind=0.0.0.0:8000 ca-cert= tls-cert= tls-key=
{{< /highlight >}}

When you get the following log-messages, it means that LoRa Server can't
connect to the application-server.

{{<highlight text>}}
INFO[0000] grpc: addrConn.resetTransport failed to create client transport: connection error: desc = "transport: dial tcp 127.0.0.1:8001: getsockopt: connection refused"; Reconnecting to {"127.0.0.1:8001" <nil>}
INFO[0001] grpc: addrConn.resetTransport failed to create client transport: connection error: desc = "transport: dial tcp 127.0.0.1:8001: getsockopt: connection refused"; Reconnecting to {"127.0.0.1:8001" <nil>}
INFO[0002] grpc: addrConn.resetTransport failed to create client transport: connection error: desc = "transport: dial tcp 127.0.0.1:8001: getsockopt: connection refused"; Reconnecting to {"127.0.0.1:8001" <nil>}
{{< /highlight >}}

## Configuration

See [Configuration]({{< relref "config.md" >}}) for details on each config option.
