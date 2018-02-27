package cmd

import (
	"html/template"
	"os"

	"github.com/brocaar/loraserver/internal/config"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)

// when updating this template, don't forget to update config.md!
const configTemplate = `[general]
# Log level
#
# debug=5, info=4, warning=3, error=2, fatal=1, panic=0
log_level={{ .General.LogLevel }}


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
dsn="{{ .PostgreSQL.DSN }}"

# Automatically apply database migrations.
#
# It is possible to apply the database-migrations by hand
# (see https://github.com/brocaar/loraserver/tree/master/migrations)
# or let LoRa App Server migrate to the latest state automatically, by using
# this setting. Make sure that you always make a backup when upgrading Lora
# App Server and / or applying migrations.
automigrate={{ .PostgreSQL.Automigrate }}


# Redis settings
#
# Please note that Redis 2.6.0+ is required.
[redis]
# Redis url (e.g. redis://user:password@hostname/0)
#
# For more information about the Redis URL format, see:
# https://www.iana.org/assignments/uri-schemes/prov/redis
url="{{ .Redis.URL }}"


# Network-server settings.
[network_server]
# Network identifier (NetID, 3 bytes) encoded as HEX (e.g. 010203)
net_id="{{ .NetworkServer.NetID }}"

# Time to wait for uplink de-duplication.
#
# This is the time that LoRa Server will wait for other gateways to receive
# the same uplink frame. Valid units are 'ms' or 's'.
# Please note that this value has influence on the uplink / downlink
# roundtrip time. Setting this value too high means LoRa Server will be
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
# This is the time that LoRa Server waits between forwarding data to the
# application-server and reading data from the queue. A higher value
# means that the application-server has more time to schedule a downlink
# queue item which can be processed within the same uplink / downlink
# transaction.
# Please note that this value has influence on the uplink / downlink
# roundtrip time. Setting this value too high means LoRa Server will be
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
  # *	AS_923
  # * AU_915_928
  # * CN_470_510
  # * CN_779_787
  # * EU_433
  # * EU_863_870
  # * IN_865_867
  # * KR_920_923
  # * US_902_928),
  name="{{ .NetworkServer.Band.Name }}"

  # Enforce 400ms dwell time
  #
  # Some band configurations define the max payload size for both dwell-time
  # limitation enabled as disabled (e.g. AS 923). In this case the
  # dwell time setting must be set to enforce the max payload size
  # given the dwell-time limitation. For band configuration where the dwell-time is
  # always enforced, setting this flag is not required.
  dwell_time_400ms={{ .NetworkServer.Band.DwellTime400ms }}

  # Enforce repeater compatibility
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

  # Class A RX1 delay
  #
  # 0=1sec, 1=1sec, ... 15=15sec. A higher value means LoRa Server has more
  # time to respond to the device as the delay between the uplink and the
  # first receive-window will be increased.
  rx1_delay={{ .NetworkServer.NetworkSettings.RX1Delay }}

  # RX1 data-rate offset
  #
  # Please consult the LoRaWAN Regional Parameters specification for valid
  # options of the configured network_server.band.name.
  rx1_dr_offset={{ .NetworkServer.NetworkSettings.RX1DROffset }}

  # RX2 data-rate (when set to -1, the default rx2 data-rate will be used)
  #
  # Please consult the LoRaWAN Regional Parameters specification for valid
  # options of the configured network_server.band.name.
  rx2_dr={{ .NetworkServer.NetworkSettings.RX2DR }}

  # Enable only a given sub-set of channels
  #
  # Use this when ony a sub-set of the by default enabled channels are being
  # used. For example when only using the first 8 channels of the US band.
  # 
  # Example:
  # enabled_uplink_channels=[0, 1, 2, 3, 4, 5, 6, 7]
  enabled_uplink_channels=[{{ range $index, $element := .NetworkServer.NetworkSettings.EnabledUplinkChannels }}{{ if $index }}, {{ end }}{{ $element }}{{ end }}]


  # Extra channels to use for ISM bands that implement the CFList
  #
  # Use this for LoRaWAN regions where it is possible to extend the by default
  # available channels with additional channels (e.g. the EU band).
  # Note: the min_dr and max_dr are currently informative, but will be enforced
  # in one of the next versions of LoRa Server!
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


  # Network-server API
  #
  # This is the network-server API that is used by LoRa App Server or other
  # custom components interacting with LoRa Server.
  [network_server.api]
  # ip:port to bind the api server
  bind="{{ .NetworkServer.API.Bind }}"

  # ca certificate used by the api server (optional)
  ca_cert="{{ .NetworkServer.API.CACert }}"

  # tls certificate used by the api server (optional)
  tls_cert="{{ .NetworkServer.API.TLSCert }}"

  # tls key used by the api server (optional)
  tls_key="{{ .NetworkServer.API.TLSKey }}"

  # Gateway API
  # 
  # This API is used by the LoRa Channel Manager component to fetch
  # channel configuration.
  [network_server.gateway.api]
  # ip:port to bind the api server
  bind="{{ .NetworkServer.Gateway.API.Bind }}"

  # CA certificate used by the api server (optional)
  ca_cert="{{ .NetworkServer.Gateway.API.CACert }}"

  # tls certificate used by the api server (optional)
  tls_cert="{{ .NetworkServer.Gateway.API.TLSCert }}"

  # tls key used by the api server (optional)
  tls_key="{{ .NetworkServer.Gateway.API.TLSKey }}"

  # JWT secret used by the gateway api server for gateway authentication / authorization
  jwt_secret="{{ .NetworkServer.Gateway.API.JWTSecret }}"

  # Gateway statistics settings.
  [network_server.gateway.stats]
  # Create non-existing gateways on receiving of stats
  #
  # When set to true, LoRa Server will create the gateway when it receives
  # statistics for a gateway that does not yet exist.
  create_gateway_on_stats={{ .NetworkServer.Gateway.Stats.CreateGatewayOnStats }}

  # Aggregation timezone
  #
  # This timezone is used for correctly aggregating the statistics (for example
  # 'Europe/Amsterdam').
  # To get the list of supported timezones by your PostgreSQL database,
  # execute the following SQL query:
  #   select * from pg_timezone_names;
  # When left blank, the default timezone of your database will be used.
  timezone="{{ .NetworkServer.Gateway.Stats.Timezone }}"

  # Aggregation intervals to use for aggregating the gateway stats
  #
  # Valid options: second, minute, hour, day, week, month, quarter, year.
  # When left empty, no statistics will be stored in the database.
  # Note, LoRa App Server expects at least "minute", "day", "hour"!
  aggregation_intervals=[{{ if .NetworkServer.Gateway.Stats.AggregationIntervals|len }}"{{ end }}{{ range $index, $element := .NetworkServer.Gateway.Stats.AggregationIntervals }}{{ if $index }}", "{{ end }}{{ $element }}{{ end }}{{ if .NetworkServer.Gateway.Stats.AggregationIntervals|len }}"{{ end }}]


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
  # Use "{{ "{{ .MAC }}" }}" as an substitution for the LoRa gateway MAC. 
  uplink_topic_template="gateway/+/rx"
  downlink_topic_template="gateway/{{ "{{ .MAC }}" }}/tx"
  stats_topic_template="gateway/+/stats"
  ack_topic_template="gateway/+/ack"

  # MQTT server (e.g. scheme://host:port where scheme is tcp, ssl or ws)
  server="{{ .NetworkServer.Gateway.Backend.MQTT.Server }}"

  # Connect with the given username (optional)
  username="{{ .NetworkServer.Gateway.Backend.MQTT.Username }}"

  # Connect with the given password (optional)
  password="{{ .NetworkServer.Gateway.Backend.MQTT.Password }}"

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


# Default join-server settings.
[join_server.default]
# hostname:port of the default join-server
#
# This API is provided by LoRa App Server.
server="{{ .JoinServer.Default.Server }}"

# ca certificate used by the default join-server client (optional)
ca_cert="{{ .JoinServer.Default.CACert }}"

# tls certificate used by the default join-server client (optional)
tls_cert="{{ .JoinServer.Default.TLSCert }}"

# tls key used by the default join-server client (optional)
tls_key="{{ .JoinServer.Default.TLSKey }}"


# Network-controller configuration.
[network_contoller]
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
	Short: "Print the LoRa Server configuration file",
	RunE: func(cmd *cobra.Command, args []string) error {
		t := template.Must(template.New("config").Parse(configTemplate))
		err := t.Execute(os.Stdout, &config.C)
		if err != nil {
			return errors.Wrap(err, "execute config template error")
		}
		return nil
	},
}
