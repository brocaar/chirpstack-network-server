package config

import (
	"time"

	"github.com/gomodule/redigo/redis"

	"github.com/brocaar/loraserver/api/geo"
	"github.com/brocaar/loraserver/api/nc"
	"github.com/brocaar/loraserver/internal/api/client/asclient"
	"github.com/brocaar/loraserver/internal/api/client/jsclient"
	"github.com/brocaar/loraserver/internal/backend"
	"github.com/brocaar/loraserver/internal/backend/gateway"
	"github.com/brocaar/loraserver/internal/common"
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
		DB          *common.DBLogger
	} `mapstructure:"postgresql"`

	Redis struct {
		URL         string        `mapstructure:"url"`
		MaxIdle     int           `mapstructure:"max_idle"`
		IdleTimeout time.Duration `mapstructure:"idle_timeout"`
		Pool        *redis.Pool
	}

	NetworkServer struct {
		NetID                lorawan.NetID
		NetIDString          string        `mapstructure:"net_id"`
		DeduplicationDelay   time.Duration `mapstructure:"deduplication_delay"`
		DeviceSessionTTL     time.Duration `mapstructure:"device_session_ttl"`
		GetDownlinkDataDelay time.Duration `mapstructure:"get_downlink_data_delay"`

		Band struct {
			Band               band.Band
			Name               band.Name
			DwellTime400ms     bool `mapstructure:"dwell_time_400ms"`
			RepeaterCompatible bool `mapstructure:"repeater_compatible"`
		}

		NetworkSettings struct {
			InstallationMargin    float64 `mapstructure:"installation_margin"`
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

			Scheduler struct {
				// SchedulerInterval is the interval in milliseconds in which the scheduler
				// sleep between runs, default to 1000
				SchedulerInterval int `mapstructure:"scheduler_interval"`
			} `mapstructure:"scheduler"`

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

		API struct {
			Bind    string
			CACert  string `mapstructure:"ca_cert"`
			TLSCert string `mapstructure:"tls_cert"`
			TLSKey  string `mapstructure:"tls_key"`
		} `mapstructure:"api"`

		Gateway struct {
			Stats struct {
				TimezoneLocation     *time.Location
				CreateGatewayOnStats bool `mapstructure:"create_gateway_on_stats"`
				Timezone             string
				AggregationIntervals []string `mapstructure:"aggregation_intervals"`
			}

			Backend struct {
				Backend backend.Gateway
				MQTT    gateway.MQTTBackendConfig
			}
		}
	} `mapstructure:"network_server"`

	GeolocationServer struct {
		Client  geo.GeolocationServerServiceClient `mapstructure:"-"`
		Server  string                             `mapstructure:"server"`
		CACert  string                             `mapstructure:"ca_cert"`
		TLSCert string                             `mapstructure:"tls_cert"`
		TLSKey  string                             `mapstructure:"tls_key"`
	} `mapstructure:"geolocation_server"`

	JoinServer struct {
		Pool jsclient.Pool

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

	ApplicationServer struct {
		Pool asclient.Pool
	}

	NetworkController struct {
		Client nc.NetworkControllerServiceClient

		Server  string
		CACert  string `mapstructure:"ca_cert"`
		TLSCert string `mapstructure:"tls_cert"`
		TLSKey  string `mapstructure:"tls_key"`
	} `mapstructure:"network_controller"`
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

// SchedulerBatchSize contains the batch size of the Class-C scheduler
var SchedulerBatchSize = 100

// ClassCDownlinkLockDuration contains the duration to lock the downlink
// Class-C transmissions after a preceeding downlink tx.
var ClassCDownlinkLockDuration = time.Second * 2

// ClassBEnqueueMargin contains the margin duration when scheduling Class-B
// messages.
var ClassBEnqueueMargin = time.Second * 5

// MulticastClassCInterval defines the interval between the gateway scheduling
// for Class-C multicast.
var MulticastClassCInterval = time.Second
