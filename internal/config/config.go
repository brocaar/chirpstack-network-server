package config

import (
	"time"

	"github.com/brocaar/loraserver/api/nc"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/band"
)

// Version defines the LoRa Server version.
var Version string

// Config defines the configuration structure.
type Config struct {
	General struct {
		LogLevel int `mapstructure:"log_level"`
	}

	PostgreSQL struct {
		DSN         string `mapstructure:"dsn"`
		Automigrate bool
	} `mapstructure:"postgresql"`

	Redis struct {
		URL         string        `mapstructure:"url"`
		MaxIdle     int           `mapstructure:"max_idle"`
		MaxActive   int           `mapstructure:"max_active"`
		IdleTimeout time.Duration `mapstructure:"idle_timeout"`
	}

	NetworkServer struct {
		NetID                lorawan.NetID
		NetIDString          string        `mapstructure:"net_id"`
		DeduplicationDelay   time.Duration `mapstructure:"deduplication_delay"`
		DeviceSessionTTL     time.Duration `mapstructure:"device_session_ttl"`
		GetDownlinkDataDelay time.Duration `mapstructure:"get_downlink_data_delay"`

		Band struct {
			Name                   band.Name
			UplinkDwellTime400ms   bool    `mapstructure:"uplink_dwell_time_400ms"`
			DownlinkDwellTime400ms bool    `mapstructure:"downlink_dwell_time_401ms"`
			UplinkMaxEIRP          float32 `mapstructure:"uplink_max_eirp"`
			RepeaterCompatible     bool    `mapstructure:"repeater_compatible"`
		}

		NetworkSettings struct {
			InstallationMargin    float64 `mapstructure:"installation_margin"`
			RXWindow              int     `mapstructure:"rx_window"`
			RX1Delay              int     `mapstructure:"rx1_delay"`
			RX1DROffset           int     `mapstructure:"rx1_dr_offset"`
			RX2DR                 int     `mapstructure:"rx2_dr"`
			RX2Frequency          int     `mapstructure:"rx2_frequency"`
			DownlinkTXPower       int     `mapstructure:"downlink_tx_power"`
			EnabledUplinkChannels []int   `mapstructure:"enabled_uplink_channels"`
			DisableMACCommands    bool    `mapstructure:"disable_mac_commands"`
			DisableADR            bool    `mapstructure:"disable_adr"`

			ExtraChannels []struct {
				Frequency int
				MinDR     int `mapstructure:"min_dr"`
				MaxDR     int `mapstructure:"max_dr"`
			} `mapstructure:"extra_channels"`

			ClassB struct {
				PingSlotDR        int `mapstructure:"ping_slot_dr"`
				PingSlotFrequency int `mapstructure:"ping_slot_frequency"`
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
				DownlinkLockDuration time.Duration `mapstructure:"downlink_lock_duration"`
			} `mapstructure:"class_c"`
		} `mapstructure:"scheduler"`

		API struct {
			Bind    string
			CACert  string `mapstructure:"ca_cert"`
			TLSCert string `mapstructure:"tls_cert"`
			TLSKey  string `mapstructure:"tls_key"`
		} `mapstructure:"api"`

		Gateway struct {
			// Deprecated
			Stats struct {
				Timezone string
			}

			Backend struct {
				Type string `mapstructure:"type"`

				MQTT struct {
					Server       string
					Username     string
					Password     string
					QOS          uint8  `mapstructure:"qos"`
					CleanSession bool   `mapstructure:"clean_session"`
					ClientID     string `mapstructure:"client_id"`
					CACert       string `mapstructure:"ca_cert"`
					TLSCert      string `mapstructure:"tls_cert"`
					TLSKey       string `mapstructure:"tls_key"`

					EventTopic           string `mapstructure:"event_topic"`
					CommandTopicTemplate string `mapstructure:"command_topic_template"`
				} `mapstructure:"mqtt"`

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
			}
		}
	} `mapstructure:"network_server"`

	GeolocationServer struct {
		Server  string `mapstructure:"server"`
		CACert  string `mapstructure:"ca_cert"`
		TLSCert string `mapstructure:"tls_cert"`
		TLSKey  string `mapstructure:"tls_key"`
	} `mapstructure:"geolocation_server"`

	JoinServer struct {
		ResolveJoinEUI      bool   `mapstructure:"resolve_join_eui"`
		ResolveDomainSuffix string `mapstructure:"resolve_domain_suffix"`

		Certificates []struct {
			JoinEUI string `mapstructure:"join_eui"`
			CaCert  string `mapstructure:"ca_cert"`
			TLSCert string `mapstructure:"tls_cert"`
			TLSKey  string `mapstructure:"tls_key"`
		} `mapstructure:"certificates"`

		Default struct {
			Server  string
			CACert  string `mapstructure:"ca_cert"`
			TLSCert string `mapstructure:"tls_cert"`
			TLSKey  string `mapstructure:"tls_key"`
		}

		KEK struct {
			Set []struct {
				Label string
				KEK   string `mapstructure:"kek"`
			}
		} `mapstructure:"kek"`
	} `mapstructure:"join_server"`

	NetworkController struct {
		Client nc.NetworkControllerServiceClient

		Server  string
		CACert  string `mapstructure:"ca_cert"`
		TLSCert string `mapstructure:"tls_cert"`
		TLSKey  string `mapstructure:"tls_key"`
	} `mapstructure:"network_controller"`

	Metrics struct {
		Timezone string `mapstructure:"timezone"`
		Redis    struct {
			AggregationIntervals []string      `mapstructure:"aggregation_intervals"`
			MinuteAggregationTTL time.Duration `mapstructure:"minute_aggregation_ttl"`
			HourAggregationTTL   time.Duration `mapstructure:"hour_aggregation_ttl"`
			DayAggregationTTL    time.Duration `mapstructure:"day_aggregation_ttl"`
			MonthAggregationTTL  time.Duration `mapstructure:"month_aggregation_ttl"`
		} `mapstructure:"redis"`
	} `mapstructure:"metrics"`
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
