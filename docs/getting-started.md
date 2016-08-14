# Getting started

!!! info
    This getting started document describes the steps needed to setup LoRa Server
    and all its requirements on Ubuntu 16.04 LTS. When using an other Linux
    distribution, you might need to adapt these steps slightly!

!!! warning
    This getting started guide does not cover setting up firewall rules! After
    setting up LoRa Server and its requirements, don't forget to configure
    your firewall rules.

## MQTT broker

LoRa Server makes use of MQTT for communication with the gateways (thought the
[LoRa Gateway Bridge](http://docs.loraserver.io/lora-gateway-bridge/)),
network-controller and applications. [Mosquitto](http://mosquitto.org/) is a
popular open-source MQTT broker. Make sure you install a recent version of
Mosquitto (the Mosquitto project provides repositories for various Linux
distributions). Ubuntu 16.04 LTS already includes a recent version which can be
installed with:

```bash
sudo apt-get install mosquitto
```

## PostgreSQL server

LoRa Server stores all persistent data into a
[PostgreSQL](http://www.postgresql.org/) database. To install PostgreSQL:

```bash
sudo apt-get install postgresql
```

### Creating a LoRa Server user and database

Start the PostgreSQL promt as the `postgres` user:

```bash
sudo -u postgres psql
```

Within the the PostgreSQL promt, enter the following queries:

```sql
-- create the loraserver user with password "dbpassword"
create role loraserver with login password 'dbpassword';

-- create the loraserver database
create database loraserver with owner loraserver;

-- exit the prompt
\q
```

To verify if the user and database have been setup correctly, try to connect
to it:

```bash
psql -h localhost -U loraserver -W dbpassword
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

# move the binary to /usr/local/bin
sudo mv loraserver /usr/local/bin
```

In order to start LoRa Server as a service, create the file
`/etc/systemd/system/loraserver.service` with as content:

```
[Unit]
Description=loraserver
After=network.target

[Service]
User=loraserver
Group=loraserver
ExecStart=/usr/local/bin/loraserver
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

In order to configure LoRa Server, we create a directory named
`/etc/systemd/system/loraserver.service.d`:

```bash
sudo mkdir /etc/systemd/system/loraserver.service.d
```

Inside this directory, put a file named `loraserver.conf`:

```
[Service]
Environment="NET_ID=010203"
Environment="BAND=EU_863_870"
Environment="HTTP_BIND=0.0.0.0:8000"
Environment="POSTGRES_DSN=postgres://loraserver:dbpassword@localhost/loraserver?sslmode=disable"
Environment="DB_AUTOMIGRATE=True"
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
level=info msg="starting LoRa Server" band="EU_863_870" net_id=010203 version=0.8.0
level=info msg="connecting to postgresql"
level=info msg="setup redis connection pool"
level=info msg="gateway/mqttpubsub: connecting to mqtt broker" server="tcp://localhost:1883"
level=info msg="application/mqttpubsub: connecting to mqtt broker" server="tcp://localhost:1883"
level=info msg="gateway/mqttpubsub: connected to mqtt server"
level=info msg="gateway/mqttpubsub: subscribing to rx topic" topic="gateway/+/rx"
level=info msg="application/mqttpubsub: connected to mqtt server"
level=info msg="application/mqttpubsub: subscribing to tx topic" topic="application/+/node/+/tx"
level=info msg="controller/mqttpubsub: connecting to mqtt broker" server="tcp://localhost:1883"
level=info msg="applying database migrations"
level=info msg="controller/mqttpubsub: connected to mqtt broker"
level=info msg="controller/mqttpubsub: subscribing to tx topic" topic="application/+/node/+/mac/tx"
level=info msg="migrations applied" count=0
level=info msg="registering json-rpc handler" path="/rpc"
level=info msg="registering gui handler" path="/"
level=info msg="starting http server" bind="0.0.0.0:8000"
```

## Configuration

In the example above, we've just touched a few configuration variables.
Run `loraserver --help` for an overview of all available variables. Note
that configuration variables can be passed as cli arguments and / or environment
variables (which we did in the above example).

See [Configuration](configuration.md) for details on each config option.

## Setting up applications and nodes

Now that your LoRa Server instance is running, it is time to create
your first application and node. Point your browser to
[http://localhost:8000/](http://localhost:8000/) for the web-interface.

After you've setup your first node, you need to provision your node with its
AppEUI, DevEUI and AppKey. See [activating nodes](activating-nodes.md)
for more details about this topic.
