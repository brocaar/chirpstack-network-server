package config

import (
	"time"

	"github.com/brocaar/chirpstack-api/go/v3/nc"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/band"
)

// Version defines the ChirpStack Network Server version.
var Version string

// Config defines the configuration structure.
type Config struct {
	General struct {
		LogLevel                  int    `mapstructure:"log_level"`
		LogToSyslog               bool   `mapstructure:"log_to_syslog"`
		GRPCDefaultResolverScheme string `mapstructure:"grpc_default_resolver_scheme"`
	} `mapstructure:"general"`

	PostgreSQL struct {
		DSN                string `mapstructure:"dsn"`
		Automigrate        bool   `mapstructure:"automigrate"`
		MaxOpenConnections int    `mapstructure:"max_open_connections"`
		MaxIdleConnections int    `mapstructure:"max_idle_connections"`
	} `mapstructure:"postgresql"`

	Redis struct {
		URL        string   `mapstructure:"url"` // deprecated
		Servers    []string `mapstructure:"servers"`
		Cluster    bool     `mapstructure:"cluster"`
		MasterName string   `mapstructure:"master_name"`
		PoolSize   int      `mapstructure:"pool_size"`
		Password   string   `mapstructure:"password"`
		Database   int      `mapstructure:"database"`
		TLSEnabled bool     `mapstructure:"tls_enabled"`
		KeyPrefix  string   `mapstructure:"key_prefix"`
	} `mapstructure:"redis"`

	NetworkServer struct {
		NetID                lorawan.NetID
		NetIDString          string        `mapstructure:"net_id"`
		DeduplicationDelay   time.Duration `mapstructure:"deduplication_delay"`
		DeviceSessionTTL     time.Duration `mapstructure:"device_session_ttl"`
		GetDownlinkDataDelay time.Duration `mapstructure:"get_downlink_data_delay"`

		Band struct {
			Name                   band.Name `mapstructure:"name"`
			UplinkDwellTime400ms   bool      `mapstructure:"uplink_dwell_time_400ms"`
			DownlinkDwellTime400ms bool      `mapstructure:"downlink_dwell_time_400ms"`
			UplinkMaxEIRP          float32   `mapstructure:"uplink_max_eirp"`
			RepeaterCompatible     bool      `mapstructure:"repeater_compatible"`
		} `mapstructure:"band"`

		NetworkSettings struct {
			InstallationMargin      float64  `mapstructure:"installation_margin"`
			RXWindow                int      `mapstructure:"rx_window"`
			RX1Delay                int      `mapstructure:"rx1_delay"`
			RX1DROffset             int      `mapstructure:"rx1_dr_offset"`
			RX2DR                   int      `mapstructure:"rx2_dr"`
			RX2Frequency            int64    `mapstructure:"rx2_frequency"`
			RX2PreferOnRX1DRLt      int      `mapstructure:"rx2_prefer_on_rx1_dr_lt"`
			RX2PreferOnLinkBudget   bool     `mapstructure:"rx2_prefer_on_link_budget"`
			GatewayPreferMinMargin  float64  `mapstructure:"gateway_prefer_min_margin"`
			DownlinkTXPower         int      `mapstructure:"downlink_tx_power"`
			EnabledUplinkChannels   []int    `mapstructure:"enabled_uplink_channels"`
			DisableMACCommands      bool     `mapstructure:"disable_mac_commands"`
			DisableADR              bool     `mapstructure:"disable_adr"`
			MaxMACCommandErrorCount int      `mapstructure:"max_mac_command_error_count"`
			ADRPlugins              []string `mapstructure:"adr_plugins"`

			ExtraChannels []struct {
				Frequency uint32 `mapstructure:"frequency"`
				MinDR     int    `mapstructure:"min_dr"`
				MaxDR     int    `mapstructure:"max_dr"`
			} `mapstructure:"extra_channels"`

			ClassB struct {
				PingSlotDR        int    `mapstructure:"ping_slot_dr"`
				PingSlotFrequency uint32 `mapstructure:"ping_slot_frequency"`
			} `mapstructure:"class_b"`

			RejoinRequest struct {
				Enabled   bool `mapstructure:"enabled"`
				MaxCountN int  `mapstructure:"max_count_n"`
				MaxTimeN  int  `mapstructure:"max_time_n"`
			} `mapstructure:"rejoin_request"`
		} `mapstructure:"network_settings"`

		Scheduler struct {
			SchedulerInterval time.Duration `mapstructure:"scheduler_interval"`

			ClassC struct {
				GatewayDownlinkLockDuration time.Duration `mapstructure:"gateway_downlink_lock_duration"`
				DeviceDownlinkLockDuration  time.Duration `mapstructure:"device_downlink_lock_duration"`
				MulticastGatewayDelay       time.Duration `mapstructure:"multicast_gateway_delay"`
			} `mapstructure:"class_c"`
		} `mapstructure:"scheduler"`

		API struct {
			Bind    string `mapstructure:"bind"`
			CACert  string `mapstructure:"ca_cert"`
			TLSCert string `mapstructure:"tls_cert"`
			TLSKey  string `mapstructure:"tls_key"`
		} `mapstructure:"api"`

		Gateway struct {
			// Deprecated
			Stats struct {
				Timezone string
			}

			CACert             string        `mapstructure:"ca_cert"`
			CAKey              string        `mapstructure:"ca_key"`
			ClientCertLifetime time.Duration `mapstructure:"client_cert_lifetime"`
			DownlinkTimeout    time.Duration `mapstructure:"downlink_timeout"`

			ForceGwsPrivate bool `mapstructure:"force_gws_private"`

			Backend struct {
				Type                 string `mapstructure:"type"`
				MultiDownlinkFeature string `mapstructure:"multi_downlink_feature"`

				MQTT struct {
					Server               string        `mapstructure:"server"`
					Username             string        `mapstructure:"username"`
					Password             string        `mapstructure:"password"`
					MaxReconnectInterval time.Duration `mapstructure:"max_reconnect_interval"`
					QOS                  uint8         `mapstructure:"qos"`
					CleanSession         bool          `mapstructure:"clean_session"`
					ClientID             string        `mapstructure:"client_id"`
					CACert               string        `mapstructure:"ca_cert"`
					TLSCert              string        `mapstructure:"tls_cert"`
					TLSKey               string        `mapstructure:"tls_key"`

					EventTopic           string `mapstructure:"event_topic"`
					CommandTopicTemplate string `mapstructure:"command_topic_template"`
				} `mapstructure:"mqtt"`

				AMQP struct {
					URL                       string `mapstructure:"url"`
					EventQueueName            string `mapstructure:"event_queue_name"`
					EventRoutingKey           string `mapstructure:"event_routing_key"`
					CommandRoutingKeyTemplate string `mapstructure:"command_routing_key_template"`
				} `mapstructure:"amqp"`

				GCPPubSub struct {
					CredentialsFile         string        `mapstructure:"credentials_file"`
					ProjectID               string        `mapstructure:"project_id"`
					UplinkTopicName         string        `mapstructure:"uplink_topic_name"`
					DownlinkTopicName       string        `mapstructure:"downlink_topic_name"`
					UplinkRetentionDuration time.Duration `mapstructure:"uplink_retention_duration"`
				} `mapstructure:"gcp_pub_sub"`

				AzureIoTHub struct {
					EventsConnectionString   string `mapstructure:"events_connection_string"`
					CommandsConnectionString string `mapstructure:"commands_connection_string"`
				} `mapstructure:"azure_iot_hub"`
			} `mapstructure:"backend"`
		} `mapstructure:"gateway"`
	} `mapstructure:"network_server"`

	JoinServer struct {
		ResolveJoinEUI      bool   `mapstructure:"resolve_join_eui"`
		ResolveDomainSuffix string `mapstructure:"resolve_domain_suffix"`

		Servers []struct {
			Server  string `mapstructure:"server"`
			JoinEUI string `mapstructure:"join_eui"`
			CACert  string `mapstructure:"ca_cert"`
			TLSCert string `mapstructure:"tls_cert"`
			TLSKey  string `mapstructure:"tls_key"`
		} `mapstructure:"servers"`

		Default struct {
			Server  string `mapstructure:"server"`
			CACert  string `mapstructure:"ca_cert"`
			TLSCert string `mapstructure:"tls_cert"`
			TLSKey  string `mapstructure:"tls_key"`
		} `mapstructure:"default"`

		KEK struct {
			Set []KEK `mapstructure:"set"`
		} `mapstructure:"kek"`
	} `mapstructure:"join_server"`

	Roaming struct {
		ResolveNetIDDomainSuffix string `mapstructure:"resolve_netid_domain_suffix"`

		API struct {
			Bind    string `mapstructure:"bind"`
			CACert  string `mapstructure:"ca_cert"`
			TLSCert string `mapstructure:"tls_cert"`
			TLSKey  string `mapstructure:"tls_key"`
		} `mapstructure:"api"`

		Servers []RoamingServer      `mapstructure:"servers"`
		Default DefaultRoamingServer `mapstructure:"default"`

		KEK struct {
			Set []KEK `mapstructure:"set"`
		} `mapstructure:"kek"`
	} `mapstructure:"roaming"`

	NetworkController struct {
		Client nc.NetworkControllerServiceClient `mapstructure:"client"`

		Server  string `mapstructure:"server"`
		CACert  string `mapstructure:"ca_cert"`
		TLSCert string `mapstructure:"tls_cert"`
		TLSKey  string `mapstructure:"tls_key"`
	} `mapstructure:"network_controller"`

	Metrics struct {
		Timezone string `mapstructure:"timezone"`

		Prometheus struct {
			EndpointEnabled    bool   `mapstructure:"endpoint_enabled"`
			Bind               string `mapstructure:"bind"`
			APITimingHistogram bool   `mapstructure:"api_timing_histogram"`
		} `mapstructure:"prometheus"`
	} `mapstructure:"metrics"`

	Monitoring struct {
		Bind                         string `mapstructure:"bind"`
		PrometheusEndpoint           bool   `mapstructure:"prometheus_endpoint"`
		PrometheusAPITimingHistogram bool   `mapstructure:"prometheus_api_timing_histogram"`
		HealthcheckEndpoint          bool   `mapstructure:"healthcheck_endpoint"`
		DeviceFrameLogMaxHistory     int64  `mapstructure:"device_frame_log_max_history"`
		GatewayFrameLogMaxHistory    int64  `mapstructure:"gateway_frame_log_max_history"`
	} `mapstructure:"monitoring"`
}

type RoamingServer struct {
	NetID                  lorawan.NetID
	NetIDString            string        `mapstructure:"net_id"`
	Async                  bool          `mapstructure:"async"`
	AsyncTimeout           time.Duration `mapstructure:"async_timeout"`
	PassiveRoaming         bool          `mapstructure:"passive_roaming"`
	PassiveRoamingLifetime time.Duration `mapstructure:"passive_roaming_lifetime"`
	PassiveRoamingKEKLabel string        `mapstructure:"passive_roaming_kek_label"`
	Server                 string        `mapstructure:"server"`
	CACert                 string        `mapstructure:"ca_cert"`
	TLSCert                string        `mapstructure:"tls_cert"`
	TLSKey                 string        `mapstructure:"tls_key"`
	Authorization          string        `mapstructure:"authorization"`
}

type DefaultRoamingServer struct {
	Enabled                bool          `mapstructure:"enabled"`
	Async                  bool          `mapstructure:"async"`
	AsyncTimeout           time.Duration `mapstructure:"async_timeout"`
	PassiveRoaming         bool          `mapstructure:"passive_roaming"`
	PassiveRoamingLifetime time.Duration `mapstructure:"passive_roaming_lifetime"`
	PassiveRoamingKEKLabel string        `mapstructure:"passive_roaming_kek_label"`
	Server                 string        `mapstructure:"server"`
	CACert                 string        `mapstructure:"ca_cert"`
	TLSCert                string        `mapstructure:"tls_cert"`
	TLSKey                 string        `mapstructure:"tls_key"`
	Authorization          string        `mapstructure:"authorization"`
}

type KEK struct {
	Label string `mapstructure:"label"`
	KEK   string `mapstructure:"kek"`
}

// SpreadFactorToRequiredSNRTable contains the required SNR to demodulate a
// LoRa frame for the given spreadfactor.
// These values are taken from the SX1276 datasheet.
var SpreadFactorToRequiredSNRTable = map[int]float64{
	6:  -5,
	7:  -7.5,
	8:  -10,
	9:  -12.5,
	10: -15,
	11: -17.5,
	12: -20,
}

// C holds the global configuration.
var C Config

// ClassBEnqueueMargin contains the margin duration when scheduling Class-B
// messages.
var ClassBEnqueueMargin = time.Second * 5

// MulticastClassCInterval defines the interval between the gateway scheduling
// for Class-C multicast.
var MulticastClassCInterval = time.Second

// Get returns the configuration.
func Get() *Config {
	return &C
}

// Set sets the configuration.
func Set(c Config) {
	C = c
}
