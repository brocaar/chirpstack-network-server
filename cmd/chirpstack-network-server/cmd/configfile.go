package cmd

import (
	"os"
	"text/template"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/brocaar/chirpstack-network-server/internal/config"
)

// when updating this template, don't forget to update config.md!
const configTemplate = `[general]
# Log level
#
# debug=5, info=4, warning=3, error=2, fatal=1, panic=0
log_level={{ .General.LogLevel }}

# Log to syslog.
#
# When set to true, log messages are being written to syslog.
log_to_syslog={{ .General.LogToSyslog }}


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
dsn="{{ .PostgreSQL.DSN }}"

# Automatically apply database migrations.
#
# It is possible to apply the database-migrations by hand
# (see https://github.com/brocaar/chirpstack-network-server/tree/master/migrations)
# or let ChirpStack Application Server migrate to the latest state automatically, by using
# this setting. Make sure that you always make a backup when upgrading ChirpStack
# Application Server and / or applying migrations.
automigrate={{ .PostgreSQL.Automigrate }}

# Max open connections.
#
# This sets the max. number of open connections that are allowed in the
# PostgreSQL connection pool (0 = unlimited).
max_open_connections={{ .PostgreSQL.MaxOpenConnections }}

# Max idle connections.
#
# This sets the max. number of idle connections in the PostgreSQL connection
# pool (0 = no idle connections are retained).
max_idle_connections={{ .PostgreSQL.MaxIdleConnections }}


# Redis settings
#
# Please note that Redis 2.6.0+ is required.
[redis]
# Redis url (e.g. redis://user:password@hostname/0)
#
# For more information about the Redis URL format, see:
# https://www.iana.org/assignments/uri-schemes/prov/redis
url="{{ .Redis.URL }}"

# Max idle connections in the pool.
max_idle={{ .Redis.MaxIdle }}

# Idle timeout.
#
# Close connections after remaining idle for this duration. If the value
# is zero, then idle connections are not closed. You should set
# the timeout to a value less than the server's timeout.
idle_timeout="{{ .Redis.IdleTimeout }}"

# Max active connections in the pool.
#
# When zero, there is no limit on the number of connections in the pool.
max_active={{ .Redis.MaxActive }}


# Network-server settings.
[network_server]
# Network identifier (NetID, 3 bytes) encoded as HEX (e.g. 010203)
net_id="{{ .NetworkServer.NetID }}"

# Time to wait for uplink de-duplication.
#
# This is the time that ChirpStack Network Server will wait for other gateways to receive
# the same uplink frame. Valid units are 'ms' or 's'.
# Please note that this value has influence on the uplink / downlink
# roundtrip time. Setting this value too high means ChirpStack Network Server will be
# unable to respond to the device within its receive-window.
deduplication_delay="{{ .NetworkServer.DeduplicationDelay }}"

# Device session expiration.
#
# The TTL value defines the time after which a device-session expires
# after no activity. Valid units are 'ms', 's', 'm', 'h'. Note that these
# values can be combined, e.g. '24h30m15s'.
device_session_ttl="{{ .NetworkServer.DeviceSessionTTL }}"

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
get_downlink_data_delay="{{ .NetworkServer.GetDownlinkDataDelay }}"


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
  name="{{ .NetworkServer.Band.Name }}"

  # Enforce 400ms dwell time.
  #
  # Some regions require the configuration of the dwell time, which will
  # limit the time-on-air to 400ms. Please refer to the LoRaWAN Regional
  # Parameters specification for more information.
  #
  # When configured and required in the configured region, ChirpStack Network Server will
  # use the TxParamSetup mac-command to communicate this to the devices.
  uplink_dwell_time_400ms={{ .NetworkServer.Band.UplinkDwellTime400ms }}
  downlink_dwell_time_400ms={{ .NetworkServer.Band.DownlinkDwellTime400ms }}

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
  uplink_max_eirp={{ .NetworkServer.Band.UplinkMaxEIRP }}

  # Enforce repeater compatibility.
  #
  # Most band configurations define the max payload size for both an optional
  # repeater encapsulation layer as for setups where a repeater will never
  # be used. The latter case increases the max payload size for some data-rates.
  # In case a repeater might used, set this flag to true.
  repeater_compatible={{ .NetworkServer.Band.RepeaterCompatible }}


  # LoRaWAN network related settings.
  [network_server.network_settings]
  # Installation margin (dB) used by the ADR engine.
  #
  # A higher number means that the network-server will keep more margin,
  # resulting in a lower data-rate but decreasing the chance that the
  # device gets disconnected because it is unable to reach one of the
  # surrounded gateways.
  installation_margin={{ .NetworkServer.NetworkSettings.InstallationMargin }}

  # RX window (Class-A).
  #
  # Set this to:
  # 0: RX1 / RX2
  # 1: RX1 only
  # 2: RX2 only
  rx_window={{ .NetworkServer.NetworkSettings.RXWindow }}

  # Class A RX1 delay
  #
  # 0=1sec, 1=1sec, ... 15=15sec. A higher value means ChirpStack Network Server has more
  # time to respond to the device as the delay between the uplink and the
  # first receive-window will be increased.
  rx1_delay={{ .NetworkServer.NetworkSettings.RX1Delay }}

  # RX1 data-rate offset
  #
  # Please consult the LoRaWAN Regional Parameters specification for valid
  # options of the configured network_server.band.name.
  rx1_dr_offset={{ .NetworkServer.NetworkSettings.RX1DROffset }}

  # RX2 data-rate
  #
  # When set to -1, the default RX2 data-rate will be used for the configured
  # LoRaWAN band.
  #
  # Please consult the LoRaWAN Regional Parameters specification for valid
  # options of the configured network_server.band.name.
  rx2_dr={{ .NetworkServer.NetworkSettings.RX2DR }}

  # RX2 frequency
  #
  # When set to -1, the default RX2 frequency will be used.
  #
  # Please consult the LoRaWAN Regional Parameters specification for valid
  # options of the configured network_server.band.name.
  rx2_frequency={{ .NetworkServer.NetworkSettings.RX2Frequency }}
  
  # Prefer RX2 on RX1 data-rate less than.
  #
  # Prefer RX2 over RX1 based on the RX1 data-rate. When the RX1 data-rate
  # is smaller than the configured value, then the Network Server will
  # first try to schedule the downlink for RX2, failing that (e.g. the gateway
  # has already a payload scheduled at the RX2 timing) it will try RX1.
  rx2_prefer_on_rx1_dr_lt={{ .NetworkServer.NetworkSettings.RX2PreferOnRX1DRLt }}
  
  # Prefer RX2 on link budget.
  #
  # When the link-budget is better for RX2 than for RX1, the Network Server will first
  # try to schedule the downlink in RX2, failing that it will try RX1.
  rx2_prefer_on_link_budget={{ .NetworkServer.NetworkSettings.RX2PreferOnLinkBudget }}

  # Downlink TX Power (dBm)
  #
  # When set to -1, the downlink TX Power from the configured band will
  # be used.
  #
  # Please consult the LoRaWAN Regional Parameters and local regulations
  # for valid and legal options. Note that the configured TX Power must be
  # supported by your gateway(s).
  downlink_tx_power={{ .NetworkServer.NetworkSettings.DownlinkTXPower }}

  # Disable mac-commands
  #
  # When set to true, ChirpStack Network Server will not handle and / or schedule any
  # mac-commands. However, it is still possible for an external controller
  # to handle and / or schedule mac-commands. This is intended for testing
  # only.
  disable_mac_commands={{ .NetworkServer.NetworkSettings.DisableMACCommands }}

  # Disable ADR
  #
  # When set, this globally disables ADR.
  disable_adr={{ .NetworkServer.NetworkSettings.DisableADR }}

  # Max mac-command error count.
  #
  # When a mac-command is nACKed for more than the configured value, then the
  # ChirpStack Network Server will stop sending this mac-command to the device.
  # This setting prevents that the Network Server will keep sending mac-commands
  # on every downlink in case of a malfunctioning device.
  max_mac_command_error_count={{ .NetworkServer.NetworkSettings.MaxMACCommandErrorCount }}

  # Enable only a given sub-set of channels
  #
  # Use this when ony a sub-set of the by default enabled channels are being
  # used. For example when only using the first 8 channels of the US band.
  # Note: when left blank, all channels will be enabled.
  # 
  # Example:
  # enabled_uplink_channels=[0, 1, 2, 3, 4, 5, 6, 7]
  enabled_uplink_channels=[{{ range $index, $element := .NetworkServer.NetworkSettings.EnabledUplinkChannels }}{{ if $index }}, {{ end }}{{ $element }}{{ end }}]


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
{{ range $index, $element := .NetworkServer.NetworkSettings.ExtraChannels }}
  [[network_server.network_settings.extra_channels]]
  frequency={{ $element.Frequency }}
  min_dr={{ $element.MinDR }}
  max_dr={{ $element.MaxDR }}
{{ end }}

  # Class B settings
  [network_server.network_settings.class_b]
  # Ping-slot data-rate.
  ping_slot_dr={{ .NetworkServer.NetworkSettings.ClassB.PingSlotDR }}

  # Ping-slot frequency (Hz)
  #
  # Set this to 0 to use the default frequency plan for the configured region
  # (which could be frequency hopping).
  ping_slot_frequency={{ .NetworkServer.NetworkSettings.ClassB.PingSlotFrequency }}


  # Rejoin-request settings
  #
  # When enabled, ChirpStack Network Server will request the device to send a rejoin-request
  # every time when one of the 2 conditions below is met (frame count or time).
  [network_server.network_settings.rejoin_request]
  # Request device to periodically send rejoin-requests
  enabled={{ .NetworkServer.NetworkSettings.RejoinRequest.Enabled }}

  # The device must send a rejoin-request type 0 at least every 2^(max_count_n + 4)
  # uplink messages. Valid values are 0 to 15.
  max_count_n={{ .NetworkServer.NetworkSettings.RejoinRequest.MaxCountN }}

  # The device must send a rejoin-request type 0 at least every 2^(max_time_n + 10)
  # seconds. Valid values are 0 to 15.
  #
  # 0  = roughly 17 minutes
  # 15 = about 1 year
  max_time_n={{ .NetworkServer.NetworkSettings.RejoinRequest.MaxTimeN }}


  # Scheduler settings
  #
  # These settings affect the multicast, Class-B and Class-C downlink queue
  # scheduler.
  [network_server.scheduler]
  # Scheduler interval
  #
  # The interval in which the downlink scheduler for multicast, Class-B and
  # Class-C runs.
  scheduler_interval="{{ .NetworkServer.Scheduler.SchedulerInterval }}"

    # Class-C settings.
    [network_server.scheduler.class_c]
    # Downlink lock duration
    #
    # Contains the duration to lock the downlink Class-C transmissions
    # after a preceeding downlink tx (per device).
    downlink_lock_duration="{{ .NetworkServer.Scheduler.ClassC.DownlinkLockDuration }}"

	# Multicast gateway delay.
	#
	# In case of a multi-gateway multicast downlink, this delay will added to
	# the transmission time of each downlink to avoid collisions between overlapping
	# gateways.
	multicast_gateway_delay="{{ .NetworkServer.Scheduler.ClassC.MulticastGatewayDelay }}"


  # Network-server API
  #
  # This is the network-server API that is used by ChirpStack Application Server or other
  # custom components interacting with ChirpStack Network Server.
  [network_server.api]
  # ip:port to bind the api server
  bind="{{ .NetworkServer.API.Bind }}"

  # ca certificate used by the api server (optional)
  ca_cert="{{ .NetworkServer.API.CACert }}"

  # tls certificate used by the api server (optional)
  tls_cert="{{ .NetworkServer.API.TLSCert }}"

  # tls key used by the api server (optional)
  tls_key="{{ .NetworkServer.API.TLSKey }}"


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
    type="{{ .NetworkServer.Gateway.Backend.Type }}"


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
    event_topic="{{ .NetworkServer.Gateway.Backend.MQTT.EventTopic }}"

    # Command topic template.
    #
    # Use:
    #   * "{{ "{{ .GatewayID }}" }}" as an substitution for the LoRa gateway ID
    #   * "{{ "{{ .CommandType }}" }}" as an substitution for the command type
    command_topic_template="{{ .NetworkServer.Gateway.Backend.MQTT.CommandTopicTemplate }}"

    # MQTT server (e.g. scheme://host:port where scheme is tcp, ssl or ws)
    server="{{ .NetworkServer.Gateway.Backend.MQTT.Server }}"

    # Connect with the given username (optional)
    username="{{ .NetworkServer.Gateway.Backend.MQTT.Username }}"

    # Connect with the given password (optional)
    password="{{ .NetworkServer.Gateway.Backend.MQTT.Password }}"

    # Maximum interval that will be waited between reconnection attempts when connection is lost.
    # Valid units are 'ms', 's', 'm', 'h'. Note that these values can be combined, e.g. '24h30m15s'.
    max_reconnect_interval="{{ .NetworkServer.Gateway.Backend.MQTT.MaxReconnectInterval }}"

    # Quality of service level
    #
    # 0: at most once
    # 1: at least once
    # 2: exactly once
    #
    # Note: an increase of this value will decrease the performance.
    # For more information: https://www.hivemq.com/blog/mqtt-essentials-part-6-mqtt-quality-of-service-levels
    qos={{ .NetworkServer.Gateway.Backend.MQTT.QOS }}

    # Clean session
    #
    # Set the "clean session" flag in the connect message when this client
    # connects to an MQTT broker. By setting this flag you are indicating
    # that no messages saved by the broker for this client should be delivered.
    clean_session={{ .NetworkServer.Gateway.Backend.MQTT.CleanSession }}

    # Client ID
    #
    # Set the client id to be used by this client when connecting to the MQTT
    # broker. A client id must be no longer than 23 characters. When left blank,
    # a random id will be generated. This requires clean_session=true.
    client_id="{{ .NetworkServer.Gateway.Backend.MQTT.ClientID }}"

    # CA certificate file (optional)
    #
    # Use this when setting up a secure connection (when server uses ssl://...)
    # but the certificate used by the server is not trusted by any CA certificate
    # on the server (e.g. when self generated).
    ca_cert="{{ .NetworkServer.Gateway.Backend.MQTT.CACert }}"

    # TLS certificate file (optional)
    tls_cert="{{ .NetworkServer.Gateway.Backend.MQTT.TLSCert }}"

    # TLS key file (optional)
    tls_key="{{ .NetworkServer.Gateway.Backend.MQTT.TLSKey }}"


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
    url="{{ .NetworkServer.Gateway.Backend.AMQP.URL }}"

    # Event queue name.
    #
    # This queue will be created when it does not yet exist and is used to
    # queue the events received from the gateway.
    event_queue_name="{{ .NetworkServer.Gateway.Backend.AMQP.EventQueueName }}"

    # Event routing key.
    #
    # This is the routing-key used for creating the queue binding.
    event_routing_key="{{ .NetworkServer.Gateway.Backend.AMQP.EventRoutingKey }}"

    # Command routing key template.
    #
    # This is the command routing-key template used when publishing gateway
    # commands.
    command_routing_key_template="{{ .NetworkServer.Gateway.Backend.AMQP.CommandRoutingKeyTemplate }}"


    # Google Cloud Pub/Sub backend.
    #
    # Use this backend when the ChirpStack Gateway Bridge is configured to connect
    # to the Google Cloud IoT Core MQTT broker (which integrates with Pub/Sub).
    [network_server.gateway.backend.gcp_pub_sub]
    # Path to the IAM service-account credentials file.
    #
    # Note: this service-account must have the following Pub/Sub roles:
    #  * Pub/Sub Editor
    credentials_file="{{ .NetworkServer.Gateway.Backend.GCPPubSub.CredentialsFile }}"

    # Google Cloud project id.
    project_id="{{ .NetworkServer.Gateway.Backend.GCPPubSub.ProjectID }}"

    # Uplink Pub/Sub topic name (to which Cloud IoT Core publishes).
    uplink_topic_name="{{ .NetworkServer.Gateway.Backend.GCPPubSub.UplinkTopicName }}"

    # Downlink Pub/Sub topic name (for publishing downlink frames).
    downlink_topic_name="{{ .NetworkServer.Gateway.Backend.GCPPubSub.DownlinkTopicName }}"

    # Uplink retention duration.
    #
    # The retention duration that ChirpStack Network Server will set on the uplink subscription.
    uplink_retention_duration="{{ .NetworkServer.Gateway.Backend.GCPPubSub.UplinkRetentionDuration }}"


    # Azure IoT Hub backend.
    #
    # Use this backend when the ChirpStack Gateway Bridge is configured to connect
    # to the Azure IoT Hub MQTT broker.
    [network_server.gateway.backend.azure_iot_hub]

    # Events connection string.
    #
    # This connection string must point to the Service Bus Queue to which the
    # IoT Hub is forwarding the (uplink) gateway events.
    events_connection_string="{{ .NetworkServer.Gateway.Backend.AzureIoTHub.EventsConnectionString }}"

    # Commands connection string.
    #
    # This connection string must point to the IoT Hub and is used by ChirpStack Network Server
    # for sending commands to the gateways.
    commands_connection_string="{{ .NetworkServer.Gateway.Backend.AzureIoTHub.CommandsConnectionString }}"


  # Geolocation settings.
  #
  # When set, ChirpStack Network Server will use the configured geolocation server to
  # resolve the location of the devices.
  [geolocation_server]
  # Server.
  #
  # The hostname:ip of the geolocation service (optional).
  server="{{ .GeolocationServer.Server }}"

  # CA certificate used by the API client (optional).
  ca_cert="{{ .GeolocationServer.CACert}}"

  # TLS certificate used by the API client (optional).
  tls_cert="{{ .GeolocationServer.TLSCert }}"

  # TLS key used by the API client (optional).
  tls_key="{{ .GeolocationServer.TLSKey }}"


# Metrics collection settings.
[metrics]
# Timezone
#
# The timezone is used for correctly aggregating the metrics (e.g. per hour,
# day or month).
# Example: "Europe/Amsterdam" or "Local" for the the system's local time zone.
timezone="{{ .Metrics.Timezone }}"


  # Metrics stored in Prometheus.
  #
  # These metrics expose information about the state of the ChirpStack Network Server
  # instance.
  [metrics.prometheus]
  # Enable Prometheus metrics endpoint.
  endpoint_enabled={{ .Metrics.Prometheus.EndpointEnabled }}

  # The ip:port to bind the Prometheus metrics server to for serving the
  # metrics endpoint.
  bind="{{ .Metrics.Prometheus.Bind }}"

  # API timing histogram.
  #
  # By setting this to true, the API request timing histogram will be enabled.
  # See also: https://github.com/grpc-ecosystem/go-grpc-prometheus#histograms
  api_timing_histogram={{ .Metrics.Prometheus.APITimingHistogram }}


# Join-server settings.
[join_server]
# Resolve JoinEUI (experimental).
# Default join-server settings.
#
# When set to true, ChirpStack Network Server will use the JoinEUI to resolve the join-server
# for the given JoinEUI. ChirpStack Network Server will fallback on the default join-server
# when resolving the JoinEUI fails.
resolve_join_eui={{ .JoinServer.ResolveJoinEUI }}

# Resolve domain suffix.
#
# This configures the domain suffix used for resolving the join-server.
resolve_domain_suffix="{{ .JoinServer.ResolveDomainSuffix }}"


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
  {{ range $index, $element := .JoinServer.Certificates }}
  [[join_server.certificates]]
  join_eui="{{ $element.JoinEUI }}"
  ca_cert="{{ $element.CACert }}"
  tls_cert="{{ $element.TLSCert }}"
  tls_key="{{ $element.TLSKey }}"
  {{ end }}

  # Default join-server settings.
  #
  # This join-server will be used when resolving the JoinEUI is set to false
  # or as a fallback when resolving the JoinEUI fails.
  [join_server.default]
  # hostname:port of the default join-server
  #
  # This API is provided by ChirpStack Application Server.
  server="{{ .JoinServer.Default.Server }}"

  # ca certificate used by the default join-server client (optional)
  ca_cert="{{ .JoinServer.Default.CACert }}"

  # tls certificate used by the default join-server client (optional)
  tls_cert="{{ .JoinServer.Default.TLSCert }}"

  # tls key used by the default join-server client (optional)
  tls_key="{{ .JoinServer.Default.TLSKey }}"


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
  {{ range $index, $element := .JoinServer.KEK.Set }}
  [[join_server.kek.set]]
  label="{{ $element.Label }}"
  kek="{{ $element.KEK }}"
  {{ end }}

  # Network-controller configuration.
  [network_controller]
  # hostname:port of the network-controller api server (optional)
  server="{{ .NetworkController.Server }}"

  # ca certificate used by the network-controller client (optional)
  ca_cert="{{ .NetworkController.CACert }}"

  # tls certificate used by the network-controller client (optional)
  tls_cert="{{ .NetworkController.TLSCert }}"

  # tls key used by the network-controller client (optional)
  tls_key="{{ .NetworkController.TLSKey }}"
`

var configCmd = &cobra.Command{
	Use:   "configfile",
	Short: "Print the ChirpStack Network Server configuration file",
	RunE: func(cmd *cobra.Command, args []string) error {
		t := template.Must(template.New("config").Parse(configTemplate))
		err := t.Execute(os.Stdout, &config.C)
		if err != nil {
			return errors.Wrap(err, "execute config template error")
		}
		return nil
	},
}
