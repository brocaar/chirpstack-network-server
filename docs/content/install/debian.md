---
title: Debian / Ubuntu
menu:
    main:
        parent: install
        weight: 2
description: Instructions on how to install ChirpStack Network Server on a Debian or Ubuntu based Linux installation.
---

# Debian / Ubuntu installation

These steps have been tested on:

* Ubuntu 18.04 (LTS)
* Debian 10 (Stretch)

## Creating an user and database

ChirpStack Network Server needs its **own** database. To create a new database,
start the PostgreSQL prompt as the `postgres` user:

{{<highlight bash>}}
sudo -u postgres psql
{{< /highlight >}}

Within the the PostgreSQL prompt, enter the following queries:

{{<highlight sql>}}
-- create the chirpstack_ns user with password 'dbpassword'
create role chirpstack_ns with login password 'dbpassword';

-- create the chirpstack_ns database
create database chirpstack_ns with owner chirpstack_ns;

-- exit the prompt
\q
{{< /highlight >}}

To verify if the user and database have been setup correctly, try to connect
to it:

{{<highlight bash>}}
psql -h localhost -U chirpstack_ns -W chirpstack_ns
{{< /highlight >}}


## ChirpStack Network Server Debian repository

ChirpStack provides pre-compiled binaries packaged as Debian (.deb)
packages. In order to activate this repository, execute the following
commands:

{{<highlight bash>}}
sudo apt-key adv --keyserver keyserver.ubuntu.com --recv-keys 1CE2AFD36DBCCA00

sudo echo "deb https://artifacts.chirpstack.io/packages/3.x/deb stable main" | sudo tee /etc/apt/sources.list.d/chirpstack.list
sudo apt-get update
{{< /highlight >}}

## Install ChirpStack Network Server

In order to install ChirpStack Network Server, execute the following command:

{{<highlight bash>}}
sudo apt-get install chirpstack-network-server
{{< /highlight >}}

After installation, modify the configuration file which is located at
`/etc/chirpstack-network-server/chirpstack-network-server.toml`.

Settings you probably want to set / change:

* `postgresql.dsn`
* `postgresql.automigrate`
* `network_server.net_id`
* `network_server.band.name`
* `metrics.timezone`

## Starting ChirpStack Network Server

How you need to (re)start and stop ChirpStack Network Server depends on if your
distribution uses init.d or systemd.

### init.d

{{<highlight bash>}}
sudo /etc/init.d/chirpstack-network-server [start|stop|restart|status]
{{< /highlight >}}

### systemd

{{<highlight bash>}}
sudo systemctl [start|stop|restart|status] chirpstack-network-server
{{< /highlight >}}

## ChirpStack Network Server log output

Now you've setup ChirpStack Network Server, it is a good time to verify that ChirpStack Network Server
is actually up-and-running. This can be done by looking at the ChirpStack Network Server
log output.

Like the previous step, which command you need to use for viewing the
log output depends on if your distribution uses init.d or systemd.

### init.d

All logs are written to `/var/log/chirpstack-network-server/chirpstack-network-server.log`.
To view and follow this logfile:

{{<highlight bash>}}
tail -f /var/log/chirpstack-network-server/chirpstack-network-server.log
{{< /highlight >}}

### systemd

{{<highlight bash>}}
journalctl -u chirpstack-network-server -f -n 50
{{< /highlight >}}


Example output:

{{<highlight text>}}
INFO[0000] starting ChirpStack Network Server                band=EU868 docs=https://www.chirpstack.io/network-server/ net_id=010203 version=3.1.0
INFO[0000] setup redis connection pool                   url=redis://localhost:6379
INFO[0000] backend/gateway: connecting to mqtt broker    server=tcp://localhost:1883
INFO[0000] connecting to application-server              ca-cert= server=127.0.0.1:8001 tls-cert= tls-key=
INFO[0000] backend/gateway: connected to mqtt server
INFO[0000] backend/gateway: subscribing to rx topic      topic=gateway/+/rx
INFO[0000] no network-controller configured
INFO[0000] starting api server                           bind=0.0.0.0:8000 ca-cert= tls-cert= tls-key=
{{< /highlight >}}

When you get the following log-messages, it means that ChirpStack Network Server can't
connect to the application-server.

{{<highlight text>}}
INFO[0000] grpc: addrConn.resetTransport failed to create client transport: connection error: desc = "transport: dial tcp 127.0.0.1:8001: getsockopt: connection refused"; Reconnecting to {"127.0.0.1:8001" <nil>}
INFO[0001] grpc: addrConn.resetTransport failed to create client transport: connection error: desc = "transport: dial tcp 127.0.0.1:8001: getsockopt: connection refused"; Reconnecting to {"127.0.0.1:8001" <nil>}
INFO[0002] grpc: addrConn.resetTransport failed to create client transport: connection error: desc = "transport: dial tcp 127.0.0.1:8001: getsockopt: connection refused"; Reconnecting to {"127.0.0.1:8001" <nil>}
{{< /highlight >}}

## Configuration

See [Configuration]({{< relref "config.md" >}}) for details on each config option.
