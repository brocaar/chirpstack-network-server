---
title: Configuration
menu:
    main:
        parent: install
        weight: 3
---

## Configuration

To list all configuration options, start `loraserver` with the `--help`
flag. This will display:

The `loraserver` binary has the following command-line flags:

```text
LoRa Server is an open-source network-server, part of the LoRa Server project
        > documentation & support: https://docs.loraserver.io/loraserver/
        > source & copyright information: https://github.com/brocaar/loraserver/

Usage:
  loraserver [flags]
  loraserver [command]

Available Commands:
  configfile  Print the LoRa Server configuration file
  help        Help about any command
  version     Print the LoRa Server version

Flags:
  -c, --config string   path to configuration file (optional)
  -h, --help            help for loraserver
      --log-level int   debug=5, info=4, error=2, fatal=1, panic=0 (default 4)

Use "loraserver [command] --help" for more information about a command.
```

### Configuration file

By default `loraserver` will look in the following order for a
configuration file at the following paths when `--config` is not:

* `loraserver.toml` (current working directory)
* `$HOME/.config/loraserver/loraserver.toml`
* `/etc/loraserver/loraserver.toml`

To load configuration from a different location, use the `--config` flag.

To generate a new configuration file `loraserver.toml`, execute the following command:

```bash
loraserver configfile > loraserver.toml
```

Note that this configuration file will be pre-filled with the current configuration
(either loaded from the paths mentioned above, or by using the `--config` flag).
This makes it possible when new fields get added to upgrade your configuration file
while preserving your old configuration. Example:

```bash
loraserver configfile --config loraserver-old.toml > loraserver-new.toml
```

Example configuration file:

```toml
[general]
# Log level
#
# debug=5, info=4, warning=3, error=2, fatal=1, panic=0
log_level=4


# PostgreSQL settings.
#
# Please note that PostgreSQL 9.5+ is required.
[postgresql]
# PostgreSQL dsn (e.g.: postgres://user:password@hostname/database?sslmode=disable).
#
# Besides using an URL (e.g. 'postgres://user:password@hostname/database?sslmode=disable')
# it is also possible to use the following format:
# 'user=loraserver dbname=loraserver sslmode=disable'.
#
# The following connection parameters are supported:
#
# * dbname - The name of the database to connect to
# * user - The user to sign in as
# * password - The user's password
# * host - The host to connect to. Values that start with / are for unix domain sockets. (default is localhost)
# * port - The port to bind to. (default is 5432)
# * sslmode - Whether or not to use SSL (default is require, this is not the default for libpq)
# * fallback_application_name - An application_name to fall back to if one isn't provided.
# * connect_timeout - Maximum wait for connection, in seconds. Zero or not specified means wait indefinitely.
# * sslcert - Cert file location. The file must contain PEM encoded data.
# * sslkey - Key file location. The file must contain PEM encoded data.
# * sslrootcert - The location of the root certificate file. The file must contain PEM encoded data.
#
# Valid values for sslmode are:
#
# * disable - No SSL
# * require - Always SSL (skip verification)
# * verify-ca - Always SSL (verify that the certificate presented by the server was signed by a trusted CA)
# * verify-full - Always SSL (verify that the certification presented by the server was signed by a trusted CA and the server host name matches the one in the certificate)
dsn="postgres://localhost/loraserver_ns?sslmode=disable"

# Automatically apply database migrations.
#
# It is possible to apply the database-migrations by hand
# (see https://github.com/brocaar/loraserver/tree/master/migrations)
# or let LoRa App Server migrate to the latest state automatically, by using
# this setting. Make sure that you always make a backup when upgrading Lora
# App Server and / or applying migrations.
automigrate=true


# Redis settings
#
# Please note that Redis 2.6.0+ is required.
[redis]
# Redis url (e.g. redis://user:password@hostname/0)
#
# For more information about the Redis URL format, see:
# https://www.iana.org/assignments/uri-schemes/prov/redis
url="redis://localhost:6379"


# Network-server settings.
[network_server]
# Network identifier (NetID, 3 bytes) encoded as HEX (e.g. 010203)
net_id="010203"

# Time to wait for uplink de-duplication.
#
# This is the time that LoRa Server will wait for other gateways to receive
# the same uplink frame. Valid units are 'ms' or 's'.
# Please note that this value has influence on the uplink / downlink
# roundtrip time. Setting this value too high means LoRa Server will be
# unable to respond to the device within its receive-window.
deduplication_delay="200ms"

# Device session expiration.
#
# The TTL value defines the time after which a device-session expires
# after no activity. Valid units are 'ms', 's', 'm', 'h'. Note that these
# values can be combined, e.g. '24h30m15s'.
device_session_ttl="744h0m0s"

# Get downlink data delay.
#
# This is the time that LoRa Server waits between forwarding data to the
# application-server and reading data from the queue. A higher value
# means that the application-server has more time to schedule a downlink
# queue item which can be processed within the same uplink / downlink
# transaction.
# Please note that this value has influence on the uplink / downlink
# roundtrip time. Setting this value too high means LoRa Server will be
# unable to respond to the device within its receive-window.
get_downlink_data_delay="100ms"


  # LoRaWAN regional band configuration.
  #
  # Note that you might want to consult the LoRaWAN Regional Parameters
  # specification for valid values that apply to your region.
  # See: https://www.lora-alliance.org/lorawan-for-developers
  [network_server.band]
  # LoRaWAN band to use.
  #
  # Valid values are:
  # *   AS_923
  # * AU_915_928
  # * CN_470_510
  # * CN_779_787
  # * EU_433
  # * EU_863_870
  # * IN_865_867
  # * KR_920_923
  # * US_902_928),
  name="EU_863_870"

  # Enforce 400ms dwell time
  #
  # Some band configurations define the max payload size for both dwell-time
  # limitation enabled as disabled (e.g. AS 923). In this case the
  # dwell time setting must be set to enforce the max payload size
  # given the dwell-time limitation. For band configuration where the dwell-time is
  # always enforced, setting this flag is not required.
  dwell_time_400ms=false

  # Enforce repeater compatibility
  #
  # Most band configurations define the max payload size for both an optional
  # repeater encapsulation layer as for setups where a repeater will never
  # be used. The latter case increases the max payload size for some data-rates.
  # In case a repeater might used, set this flag to true.
  repeater_compatible=false


  # LoRaWAN network related settings.
  [network_server.network_settings]
  # Installation margin (dB) used by the ADR engine.
  #
  # A higher number means that the network-server will keep more margin,
  # resulting in a lower data-rate but decreasing the chance that the
  # device gets disconnected because it is unable to reach one of the
  # surrounded gateways.
  installation_margin=10

  # Class A RX1 delay
  #
  # 0=1sec, 1=1sec, ... 15=15sec. A higher value means LoRa Server has more
  # time to respond to the device as the delay between the uplink and the
  # first receive-window will be increased.
  rx1_delay=1

  # RX1 data-rate offset
  #
  # Please consult the LoRaWAN Regional Parameters specification for valid
  # options of the configured network_server.band.name.
  rx1_dr_offset=0

  # RX2 data-rate
  #
  # When set to -1, the default RX2 data-rate will be used for the configured
  # LoRaWAN band.
  #
  # Please consult the LoRaWAN Regional Parameters specification for valid
  # options of the configured network_server.band.name.
  rx2_dr=-1

  # RX2 frequency
  #
  # When set to -1, the default RX2 frequency will be used.
  #
  # Please consult the LoRaWAN Regional Parameters specification for valid
  # options of the configured network_server.band.name.
  rx2_frequency=-1

  # Enable only a given sub-set of channels
  #
  # Use this when ony a sub-set of the by default enabled channels are being
  # used. For example when only using the first 8 channels of the US band.
  #
  # Example:
  # enabled_uplink_channels=[0, 1, 2, 3, 4, 5, 6, 7]
  enabled_uplink_channels=[]


  # Extra channel configuration.
  #
  # Use this for LoRaWAN regions where it is possible to extend the by default
  # available channels with additional channels (e.g. the EU band).
  # The first 5 channels will be configured as part of the OTAA join-response
  # (using the CFList field).
  # The other channels (or channel / data-rate changes) will be (re)configured
  # using the NewChannelReq mac-command.
  #
  # Example:
  # [[network_server.network_settings.extra_channels]]
  # frequency=867100000
  # min_dr=0
  # max_dr=5

  # [[network_server.network_settings.extra_channels]]
  # frequency=867300000
  # min_dr=0
  # max_dr=5

  # [[network_server.network_settings.extra_channels]]
  # frequency=867500000
  # min_dr=0
  # max_dr=5

  # [[network_server.network_settings.extra_channels]]
  # frequency=867700000
  # min_dr=0
  # max_dr=5

  # [[network_server.network_settings.extra_channels]]
  # frequency=867900000
  # min_dr=0
  # max_dr=5


  # Class B settings
  [network_server.network_settings.class_b]
  # Ping-slot data-rate.
  ping_slot_dr=0

  # Ping-slot frequency (Hz)
  #
  # Set this to 0 to use the default frequency plan for the configured region
  # (which could be frequency hopping).
  ping_slot_frequency=0


  # Network-server API
  #
  # This is the network-server API that is used by LoRa App Server or other
  # custom components interacting with LoRa Server.
  [network_server.api]
  # ip:port to bind the api server
  bind="0.0.0.0:8000"

  # ca certificate used by the api server (optional)
  ca_cert=""

  # tls certificate used by the api server (optional)
  tls_cert=""

  # tls key used by the api server (optional)
  tls_key=""

  # Gateway API
  #
  # This API is used by the LoRa Channel Manager component to fetch
  # channel configuration.
  [network_server.gateway.api]
  # ip:port to bind the api server
  bind="0.0.0.0:8002"

  # CA certificate used by the api server (optional)
  ca_cert=""

  # tls certificate used by the api server (optional)
  tls_cert=""

  # tls key used by the api server (optional)
  tls_key=""

  # JWT secret used by the gateway api server for gateway authentication / authorization
  jwt_secret=""

  # Gateway statistics settings.
  [network_server.gateway.stats]
  # Create non-existing gateways on receiving of stats
  #
  # When set to true, LoRa Server will create the gateway when it receives
  # statistics for a gateway that does not yet exist.
  create_gateway_on_stats=true

  # Aggregation timezone
  #
  # This timezone is used for correctly aggregating the statistics (for example
  # 'Europe/Amsterdam').
  # To get the list of supported timezones by your PostgreSQL database,
  # execute the following SQL query:
  #   select * from pg_timezone_names;
  # When left blank, the default timezone of your database will be used.
  timezone=""

  # Aggregation intervals to use for aggregating the gateway stats
  #
  # Valid options: second, minute, hour, day, week, month, quarter, year.
  # When left empty, no statistics will be stored in the database.
  # Note, LoRa App Server expects at least "minute", "day", "hour"!
  aggregation_intervals=["minute", "hour", "day"]


  # MQTT gateway backend settings.
  #
  # This is the backend communicating with the LoRa gateways over a MQTT broker.
  [network_server.gateway.backend.mqtt]
  # MQTT topic templates for the different MQTT topics.
  #
  # The meaning of these topics are documented at:
  # https://docs.loraserver.io/lora-gateway-bridge/use/data/
  #
  # The default values match the default expected configuration of the
  # LoRa Gateway Bridge MQTT backend. Therefore only change these values when
  # absolutely needed.
  # Use "{{ .MAC }}" as an substitution for the LoRa gateway MAC.
  uplink_topic_template="gateway/+/rx"
  downlink_topic_template="gateway/{{ .MAC }}/tx"
  stats_topic_template="gateway/+/stats"
  ack_topic_template="gateway/+/ack"

  # MQTT server (e.g. scheme://host:port where scheme is tcp, ssl or ws)
  server="tcp://localhost:1883"

  # Connect with the given username (optional)
  username=""

  # Connect with the given password (optional)
  password=""

  # Quality of service level
  #
  # 0: at most once
  # 1: at least once
  # 2: exactly once
  #
  # Note: an increase of this value will decrease the performance.
  # For more information: https://www.hivemq.com/blog/mqtt-essentials-part-6-mqtt-quality-of-service-levels
  qos=0

  # Clean session
  #
  # Set the "clean session" flag in the connect message when this client
  # connects to an MQTT broker. By setting this flag you are indicating
  # that no messages saved by the broker for this client should be delivered.
  clean_session=true

  # Client ID
  #
  # Set the client id to be used by this client when connecting to the MQTT
  # broker. A client id must be no longer than 23 characters. When left blank,
  # a random id will be generated. This requires clean_session=true.
  client_id=""

  # CA certificate file (optional)
  #
  # Use this when setting up a secure connection (when server uses ssl://...)
  # but the certificate used by the server is not trusted by any CA certificate
  # on the server (e.g. when self generated).
  ca_cert=""

  # TLS certificate file (optional)
  tls_cert=""

  # TLS key file (optional)
  tls_key=""


# Default join-server settings.
[join_server.default]
# hostname:port of the default join-server
#
# This API is provided by LoRa App Server.
server="http://localhost:8003"

# ca certificate used by the default join-server client (optional)
ca_cert=""

# tls certificate used by the default join-server client (optional)
tls_cert=""

# tls key used by the default join-server client (optional)
tls_key=""


# Network-controller configuration.
[network_controller]
# hostname:port of the network-controller api server (optional)
server=""

# ca certificate used by the network-controller client (optional)
ca_cert=""

# tls certificate used by the network-controller client (optional)
tls_cert=""

# tls key used by the network-controller client (optional)
tls_key=""
```

### Securing the network-server API

In order to protect the network-server API (`network_server.api`) against
unauthorized access and to encrypt all communication, it is advised to use
TLS certificates. Once the `ca_cert`, `tls_cert` and `tls_key` are set,
the API will enforce client certificate validation on all incoming connections.
This means that when configuring this network-server instance in LoRa App Server,
you must provide the CA and TLS client certificate. See also LoRa App Server
[network-server management](https://docs.loraserver.io/lora-app-server/use/network-servers/).

See [https://github.com/brocaar/loraserver-certificates](https://github.com/brocaar/loraserver-certificates)
for a set of scripts to generate such certificates.

### Join-server API configuration

In the current implementation LoRa Server uses a fixed join-server URL
(provided by LoRa App Server) which is used as a join-server backend (`join_server.default`).

In case this endpoint is secured using a TLS certificate and expects a client
certificate, you must set `ca_cert`, `tls_cert` and `tls_key`.
Also dont forget to change `server` from `http://...` to `https://...`.

See [https://github.com/brocaar/loraserver-certificates](https://github.com/brocaar/loraserver-certificates)
for a set of scripts to generate such certificates.
