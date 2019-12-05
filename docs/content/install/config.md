---
title: Configuration
menu:
    main:
        parent: install
        weight: 3
toc: false
description: Instructions and examples how to configure the ChirpStack Network Server service.
---

# Configuration

To list all configuration options, start `chirpstack-network-server` with the `--help`
flag. This will display:

The `chirpstack-network-server` binary has the following command-line flags:

{{<highlight text>}}
ChirpStack Network Server is an open-source network-server, part of the ChirpStack Network Server stack
        > documentation & support: https://www.chirpstack.io/network-server/
        > source & copyright information: https://github.com/brocaar/network-server/

Usage:
  chirpstack-network-server [flags]
  chirpstack-network-server [command]

Available Commands:
  configfile  Print the ChirpStack Network Server configuration file
  help        Help about any command
  version     Print the ChirpStack Network Server version

Flags:
  -c, --config string   path to configuration file (optional)
  -h, --help            help for chirpstack-network-server
      --log-level int   debug=5, info=4, error=2, fatal=1, panic=0 (default 4)

Use "chirpstack-network-server [command] --help" for more information about a command.
{{< /highlight >}}

## Configuration file

By default `chirpstack-network-server` will look in the following order for a
configuration file at the following paths when `--config` is not:

* `chirpstack-network-server.toml` (current working directory)
* `$HOME/.config/chirpstack-network-server/chirpstack-network-server.toml`
* `/etc/chirpstack-network-server/chirpstack-network-server.toml`

To load configuration from a different location, use the `--config` flag.

To generate a new configuration file `chirpstack-network-server.toml`, execute the following command:

{{<highlight bash>}}
chirpstack-network-server configfile > chirpstack-network-server.toml
{{< /highlight >}}

Note that this configuration file will be pre-filled with the current configuration
(either loaded from the paths mentioned above, or by using the `--config` flag).
This makes it possible when new fields get added to upgrade your configuration file
while preserving your old configuration. Example:

{{<highlight bash>}}
chirpstack-network-server configfile --config chirpstack-network-server-old.toml > chirpstack-network-server-new.toml
{{< /highlight >}}

Example configuration file:

{{<highlight toml>}}
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
# 'user=chirpstack_ns dbname=chirpstack_ns sslmode=disable'.
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
dsn="postgres://localhost/chirpstack_ns?sslmode=disable"

# Automatically apply database migrations.
#
# It is possible to apply the database-migrations by hand
# (see https://github.com/brocaar/chirpstack-network-server/tree/master/migrations)
# or let ChirpStack Application Server migrate to the latest state automatically, by using
# this setting. Make sure that you always make a backup when upgrading ChirpStack
# Application Server and / or applying migrations.
automigrate=true

# Max open connections.
#
# This sets the max. number of open connections that are allowed in the
# PostgreSQL connection pool (0 = unlimited).
max_open_connections=0

# Max idle connections.
#
# This sets the max. number of idle connections in the PostgreSQL connection
# pool (0 = no idle connections are retained).
max_idle_connections=2


# Redis settings
#
# Please note that Redis 2.6.0+ is required.
[redis]
# Redis url (e.g. redis://user:password@hostname/0)
#
# For more information about the Redis URL format, see:
# https://www.iana.org/assignments/uri-schemes/prov/redis
url="redis://localhost:6379"

# Max idle connections in the pool.
max_idle=10

# Idle timeout.
#
# Close connections after remaining idle for this duration. If the value
# is zero, then idle connections are not closed. You should set
# the timeout to a value less than the server's timeout.
idle_timeout="5m0s"

# Max active connections in the pool.
#
# When zero, there is no limit on the number of connections in the pool.
max_active=0


# Network-server settings.
[network_server]
# Network identifier (NetID, 3 bytes) encoded as HEX (e.g. 010203)
net_id="000000"

# Time to wait for uplink de-duplication.
#
# This is the time that ChirpStack Network Server will wait for other gateways to receive
# the same uplink frame. Valid units are 'ms' or 's'.
# Please note that this value has influence on the uplink / downlink
# roundtrip time. Setting this value too high means ChirpStack Network Server will be
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
# This is the time that ChirpStack Network Server waits between forwarding data to the
# application-server and reading data from the queue. A higher value
# means that the application-server has more time to schedule a downlink
# queue item which can be processed within the same uplink / downlink
# transaction.
# Please note that this value has influence on the uplink / downlink
# roundtrip time. Setting this value too high means ChirpStack Network Server will be
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
  # * AS_923
  # * AU_915_928
  # * CN_470_510
  # * CN_779_787
  # * EU_433
  # * EU_863_870
  # * IN_865_867
  # * KR_920_923
  # * RU_864_870
  # * US_902_928
  name="EU_863_870"

  # Enforce 400ms dwell time.
  #
  # Some regions require the configuration of the dwell time, which will
  # limit the time-on-air to 400ms. Please refer to the LoRaWAN Regional
  # Parameters specification for more information.
  #
  # When configured and required in the configured region, ChirpStack Network Server will
  # use the TxParamSetup mac-command to communicate this to the devices.
  uplink_dwell_time_400ms=false
  downlink_dwell_time_400ms=false

  # Uplink max. EIRP.
  #
  # This defines the maximum allowed device EIRP which must be configured
  # for some regions. Please refer the LoRaWAN Regional Parameters specification
  # for more information. Set this to -1 to use the default value for this
  # region.
  #
  # When required in the configured region, ChirpStack Network Server will use the
  # TxParamSetup mac-command to communicate this to the devices.
  # For regions where the TxParamSetup mac-command is not implemented, this
  # setting is ignored.
  uplink_max_eirp=-1

  # Enforce repeater compatibility.
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

  # RX window (Class-A).
  #
  # Set this to:
  # 0: RX1 / RX2
  # 1: RX1 only
  # 2: RX2 only
  rx_window=0

  # Class A RX1 delay
  #
  # 0=1sec, 1=1sec, ... 15=15sec. A higher value means ChirpStack Network Server has more
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

  # Prefer RX2 on RX1 data-rate less than.
  #
  # Prefer RX2 over RX1 based on the RX1 data-rate. When the RX1 data-rate
  # is smaller than the configured value, then the Network Server will
  # first try to schedule the downlink for RX2, failing that (e.g. the gateway
  # has already a payload scheduled at the RX2 timing) it will try RX1.
  rx2_prefer_on_rx1_dr_lt=0

  # Prefer RX2 on link budget.
  #
  # When the link-budget is better for RX2 than for RX1, the Network Server will first
  # try to schedule the downlink in RX2, failing that it will try RX1.
  rx2_prefer_on_link_budget=false

  # Downlink TX Power (dBm)
  #
  # When set to -1, the downlink TX Power from the configured band will
  # be used.
  #
  # Please consult the LoRaWAN Regional Parameters and local regulations
  # for valid and legal options. Note that the configured TX Power must be
  # supported by your gateway(s).
  downlink_tx_power=-1

  # Disable mac-commands
  #
  # When set to true, ChirpStack Network Server will not handle and / or schedule any
  # mac-commands. However, it is still possible for an external controller
  # to handle and / or schedule mac-commands. This is intended for testing
  # only.
  disable_mac_commands=false

  # Disable ADR
  #
  # When set, this globally disables ADR.
  disable_adr=false

  # Max mac-command error count.
  #
  # When a mac-command is nACKed for more than the configured value, then the
  # ChirpStack Network Server will stop sending this mac-command to the device.
  # This setting prevents that the Network Server will keep sending mac-commands
  # on every downlink in case of a malfunctioning device.
  max_mac_command_error_count=3

  # Enable only a given sub-set of channels
  #
  # Use this when ony a sub-set of the by default enabled channels are being
  # used. For example when only using the first 8 channels of the US band.
  # Note: when left blank, all channels will be enabled.
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


  # Rejoin-request settings
  #
  # When enabled, ChirpStack Network Server will request the device to send a rejoin-request
  # every time when one of the 2 conditions below is met (frame count or time).
  [network_server.network_settings.rejoin_request]
  # Request device to periodically send rejoin-requests
  enabled=false

  # The device must send a rejoin-request type 0 at least every 2^(max_count_n + 4)
  # uplink messages. Valid values are 0 to 15.
  max_count_n=0

  # The device must send a rejoin-request type 0 at least every 2^(max_time_n + 10)
  # seconds. Valid values are 0 to 15.
  #
  # 0  = roughly 17 minutes
  # 15 = about 1 year
  max_time_n=0


  # Scheduler settings
  #
  # These settings affect the multicast, Class-B and Class-C downlink queue
  # scheduler.
  [network_server.scheduler]
  # Scheduler interval
  #
  # The interval in which the downlink scheduler for multicast, Class-B and
  # Class-C runs.
  scheduler_interval="1s"

    # Class-C settings.
    [network_server.scheduler.class_c]
    # Downlink lock duration
    #
    # Contains the duration to lock the downlink Class-C transmissions
    # after a preceeding downlink tx (per device).
    downlink_lock_duration="2s"

        # Multicast gateway delay.
        #
        # In case of a multi-gateway multicast downlink, this delay will added to
        # the transmission time of each downlink to avoid collisions between overlapping
        # gateways.
        multicast_gateway_delay="2s"


  # Network-server API
  #
  # This is the network-server API that is used by ChirpStack Application Server or other
  # custom components interacting with ChirpStack Network Server.
  [network_server.api]
  # ip:port to bind the api server
  bind="0.0.0.0:8000"

  # ca certificate used by the api server (optional)
  ca_cert=""

  # tls certificate used by the api server (optional)
  tls_cert=""

  # tls key used by the api server (optional)
  tls_key=""


  # Backend defines the gateway backend settings.
  #
  # The gateway backend handles the communication with the gateway(s) part of
  # the LoRaWAN network.
  [network_server.gateway.backend]
    # Backend
    #
    # This defines the backend to use for the communication with the gateways.
    # Use the section name of one of the following gateway backends.
    # Valid options are:
    #  * mqtt
    #  * amqp
    #  * gcp_pub_sub
    #  * azure_iot_hub
    type="mqtt"


    # MQTT gateway backend settings.
    #
    # This is the backend communicating with the LoRa gateways over a MQTT broker.
    [network_server.gateway.backend.mqtt]
    # MQTT topic templates for the different MQTT topics.
    #
    # The meaning of these topics are documented at:
    # https://www.chirpstack.io/gateway-bridge/
    #
    # The default values match the default expected configuration of the
    # ChirpStack Gateway Bridge MQTT backend. Therefore only change these values when
    # absolutely needed.

    # Event topic template.
    event_topic="gateway/+/event/+"

    # Command topic template.
    #
    # Use:
    #   * "{{ .GatewayID }}" as an substitution for the LoRa gateway ID
    #   * "{{ .CommandType }}" as an substitution for the command type
    command_topic_template="gateway/{{ .GatewayID }}/command/{{ .CommandType }}"

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


    # AMQP / RabbitMQ.
    #
    # Use this backend when the ChirpStack Gateway Bridge is configured to connect
    # to RabbitMQ using the MQTT plugin. See for more details about this plugin:
    # https://www.rabbitmq.com/mqtt.html
    [network_server.gateway.backend.amqp]
    # Server URL.
    #
    # See for a specification of all the possible options:
    # https://www.rabbitmq.com/uri-spec.html
    url="amqp://guest:guest@localhost:5672"

    # Event queue name.
    #
    # This queue will be created when it does not yet exist and is used to
    # queue the events received from the gateway.
    event_queue_name="gateway-events"

    # Event routing key.
    #
    # This is the routing-key used for creating the queue binding.
    event_routing_key="gateway.*.event.*"

    # Command routing key template.
    #
    # This is the command routing-key template used when publishing gateway
    # commands.
    command_routing_key_template="gateway.{{ .GatewayID }}.command.{{ .CommandType }}"


    # Google Cloud Pub/Sub backend.
    #
    # Use this backend when the ChirpStack Gateway Bridge is configured to connect
    # to the Google Cloud IoT Core MQTT broker (which integrates with Pub/Sub).
    [network_server.gateway.backend.gcp_pub_sub]
    # Path to the IAM service-account credentials file.
    #
    # Note: this service-account must have the following Pub/Sub roles:
    #  * Pub/Sub Editor
    credentials_file=""

    # Google Cloud project id.
    project_id=""

    # Uplink Pub/Sub topic name (to which Cloud IoT Core publishes).
    uplink_topic_name=""

    # Downlink Pub/Sub topic name (for publishing downlink frames).
    downlink_topic_name=""

    # Uplink retention duration.
    #
    # The retention duration that ChirpStack Network Server will set on the uplink subscription.
    uplink_retention_duration="24h0m0s"


    # Azure IoT Hub backend.
    #
    # Use this backend when the ChirpStack Gateway Bridge is configured to connect
    # to the Azure IoT Hub MQTT broker.
    [network_server.gateway.backend.azure_iot_hub]

    # Events connection string.
    #
    # This connection string must point to the Service Bus Queue to which the
    # IoT Hub is forwarding the (uplink) gateway events.
    events_connection_string=""

    # Commands connection string.
    #
    # This connection string must point to the IoT Hub and is used by ChirpStack Network Server
    # for sending commands to the gateways.
    commands_connection_string=""


  # Geolocation settings.
  #
  # When set, ChirpStack Network Server will use the configured geolocation server to
  # resolve the location of the devices.
  [geolocation_server]
  # Server.
  #
  # The hostname:ip of the geolocation service (optional).
  server=""

  # CA certificate used by the API client (optional).
  ca_cert=""

  # TLS certificate used by the API client (optional).
  tls_cert=""

  # TLS key used by the API client (optional).
  tls_key=""


# Metrics collection settings.
[metrics]
# Timezone
#
# The timezone is used for correctly aggregating the metrics (e.g. per hour,
# day or month).
# Example: "Europe/Amsterdam" or "Local" for the the system's local time zone.
timezone="Local"

  # Metrics stored in Redis.
  #
  # The following metrics are stored in Redis:
  # * gateway statistics
  [metrics.redis]
  # Aggregation intervals
  #
  # The intervals on which to aggregate. Available options are:
  # 'MINUTE', 'HOUR', 'DAY', 'MONTH'.
  aggregation_intervals=["MINUTE", "HOUR", "DAY", "MONTH"]

  # Aggregated statistics storage duration.
  minute_aggregation_ttl="2h0m0s"
  hour_aggregation_ttl="48h0m0s"
  day_aggregation_ttl="2160h0m0s"
  month_aggregation_ttl="17520h0m0s"


  # Metrics stored in Prometheus.
  #
  # These metrics expose information about the state of the ChirpStack Network Server
  # instance.
  [metrics.prometheus]
  # Enable Prometheus metrics endpoint.
  endpoint_enabled=false

  # The ip:port to bind the Prometheus metrics server to for serving the
  # metrics endpoint.
  bind=""

  # API timing histogram.
  #
  # By setting this to true, the API request timing histogram will be enabled.
  # See also: https://github.com/grpc-ecosystem/go-grpc-prometheus#histograms
  api_timing_histogram=false


# Join-server settings.
[join_server]
# Resolve JoinEUI (experimental).
# Default join-server settings.
#
# When set to true, ChirpStack Network Server will use the JoinEUI to resolve the join-server
# for the given JoinEUI. ChirpStack Network Server will fallback on the default join-server
# when resolving the JoinEUI fails.
resolve_join_eui=false

# Resolve domain suffix.
#
# This configures the domain suffix used for resolving the join-server.
resolve_domain_suffix=".joineuis.lora-alliance.org"


  # Join-server certificates.
  #
  # Example:
  # [[join_server.certificates]]
  # # JoinEUI.
  # #
  # # The JoinEUI of the joinserver to to use the certificates for.
  # join_eui="0102030405060708"

  # # CA certificate (optional).
  # #
  # # Set this to validate the join-server server certificate (e.g. when the
  # # certificate was self-signed).
  # ca_cert="/path/to/ca.pem"

  # # TLS client-certificate (optional).
  # #
  # # Set this to enable client-certificate authentication with the join-server.
  # tls_cert="/path/to/tls_cert.pem"

  # # TLS client-certificate key (optional).
  # #
  # # Set this to enable client-certificate authentication with the join-server.
  # tls_key="/path/to/tls_key.pem"


  # Default join-server settings.
  #
  # This join-server will be used when resolving the JoinEUI is set to false
  # or as a fallback when resolving the JoinEUI fails.
  [join_server.default]
  # hostname:port of the default join-server
  #
  # This API is provided by ChirpStack Application Server.
  server="http://localhost:8003"

  # ca certificate used by the default join-server client (optional)
  ca_cert=""

  # tls certificate used by the default join-server client (optional)
  tls_cert=""

  # tls key used by the default join-server client (optional)
  tls_key=""


  # Join-server KEK set.
  #
  # These KEKs (Key Encryption Keys) are used to decrypt the network related
  # session-keys received from the join-server on a (re)join-accept.
  # Please refer to the LoRaWAN Backend Interface specification
  # 'Key Transport Security' section for more information.
  #
  # Example (the [[join_server.kek.set]] can be repeated):
  # [[join_server.kek.set]]
  # # KEK label.
  # label="000000"

  # # Key Encryption Key.
  # kek="01020304050607080102030405060708"


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
{{< /highlight >}}

## Securing the Network Server API

In order to protect the Network Server API (`network_server.api`) against
unauthorized access and to encrypt all communication, it is advised to use
TLS certificates. Once the `ca_cert`, `tls_cert` and `tls_key` are set,
the API will enforce client certificate validation on all incoming connections.
This means that when configuring this Network Server instance in [ChirpStack Application Server](/application-server/)
you must provide the CA and TLS client certificate. See also ChirpStack Application Server
[Network Server Management](/application-server/use/network-servers/).

See [https://github.com/brocaar/chirpstack-certificates](https://github.com/brocaar/chirpstack-certificates)
for a set of scripts to generate such certificates.

## Join Server API configuration

In the current implementation ChirpStack Network Server uses a fixed join-server URL
(provided by ChirpStack Application Server) which is used as a Soin Server backend (`join_server.default`).

In case this endpoint is secured using a TLS certificate and expects a client
certificate, you must set `ca_cert`, `tls_cert` and `tls_key`.
Also don't forget to change `server` from `http://...` to `https://...`.

See [https://github.com/brocaar/chirpstack-certificates](https://github.com/brocaar/chirpstack-certificates)
for a set of scripts to generate such certificates.
