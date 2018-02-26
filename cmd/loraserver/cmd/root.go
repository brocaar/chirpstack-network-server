package cmd

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/brocaar/loraserver/internal/config"
	"github.com/brocaar/lorawan/band"
)

var cfgFile string
var version string

var bands = []string{
	string(band.AS_923),
	string(band.AU_915_928),
	string(band.CN_470_510),
	string(band.CN_779_787),
	string(band.EU_433),
	string(band.EU_863_870),
	string(band.IN_865_867),
	string(band.KR_920_923),
	string(band.US_902_928),
}

var rootCmd = &cobra.Command{
	Use:   "loraserver",
	Short: "LoRa Server network-server",
	Long: `LoRa Server is an open-source network-server, part of the LoRa Server project
	> documentation & support: https://docs.loraserver.io/loraserver/
	> source & copyright information: https://github.com/brocaar/loraserver/`,
	RunE: run,
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "path to configuration file (optional)")
	rootCmd.PersistentFlags().Int("log-level", 4, "debug=5, info=4, error=2, fatal=1, panic=0")

	// for backwards compatibility
	rootCmd.PersistentFlags().String("net-id", "010203", "")
	rootCmd.PersistentFlags().String("band", "EU_863_870", "")
	rootCmd.PersistentFlags().Bool("band-dwell-time-400ms", false, "")
	rootCmd.PersistentFlags().Bool("band-repeater-compatible", false, "")
	rootCmd.PersistentFlags().String("ca-cert", "", "")
	rootCmd.PersistentFlags().String("tls-cert", "", "")
	rootCmd.PersistentFlags().String("tls-key", "", "")
	rootCmd.PersistentFlags().String("bind", "0.0.0.0:8000", "")
	rootCmd.PersistentFlags().String("gw-server-ca-cert", "", "")
	rootCmd.PersistentFlags().String("gw-server-tls-cert", "", "")
	rootCmd.PersistentFlags().String("gw-server-tls-key", "", "")
	rootCmd.PersistentFlags().String("gw-server-jwt-secret", "", "")
	rootCmd.PersistentFlags().String("gw-server-bind", "0.0.0.0:8002", "")
	rootCmd.PersistentFlags().String("redis-url", "redis://localhost:6379", "")
	rootCmd.PersistentFlags().String("postgres-dsn", "postgres://localhost/loraserver_ns?sslmode=disable", "")
	rootCmd.PersistentFlags().Bool("db-automigrate", true, "")
	rootCmd.PersistentFlags().String("gw-mqtt-server", "tcp://localhost:1883", "")
	rootCmd.PersistentFlags().String("gw-mqtt-username", "", "")
	rootCmd.PersistentFlags().String("gw-mqtt-password", "", "")
	rootCmd.PersistentFlags().String("gw-mqtt-ca-cert", "", "")
	rootCmd.PersistentFlags().String("gw-mqtt-tls-cert", "", "")
	rootCmd.PersistentFlags().String("gw-mqtt-tls-key", "", "")
	rootCmd.PersistentFlags().String("nc-server", "", "")
	rootCmd.PersistentFlags().String("nc-ca-cert", "", "")
	rootCmd.PersistentFlags().String("nc-tls-cert", "", "")
	rootCmd.PersistentFlags().String("nc-tls-key", "", "")
	rootCmd.PersistentFlags().Duration("deduplication-delay", 200*time.Millisecond, "")
	rootCmd.PersistentFlags().Duration("get-downlink-data-delay", 100*time.Millisecond, "")
	rootCmd.PersistentFlags().StringSlice("gw-stats-aggregation-intervals", []string{"minute", "hour", "day"}, "")
	rootCmd.PersistentFlags().String("timezone", "", "")
	rootCmd.PersistentFlags().Bool("gw-create-on-stats", true, "")
	rootCmd.PersistentFlags().IntSlice("extra-frequencies", nil, "")
	rootCmd.PersistentFlags().String("enable-uplink-channels", "", "")
	rootCmd.PersistentFlags().Duration("node-session-ttl", time.Hour*24*31, "")
	rootCmd.PersistentFlags().String("js-server", "http://localhost:8003", "")
	rootCmd.PersistentFlags().String("js-ca-cert", "", "")
	rootCmd.PersistentFlags().String("js-tls-cert", "", "")
	rootCmd.PersistentFlags().String("js-tls-key", "", "")
	rootCmd.PersistentFlags().Float64("installation-margin", 10, "")
	rootCmd.PersistentFlags().Int("rx1-delay", 1, "")
	rootCmd.PersistentFlags().Int("rx1-dr-offset", 0, "")
	rootCmd.PersistentFlags().Int("rx2-dr", -1, "")

	hidden := []string{
		"net-id", "band", "band-dwell-time-400ms", "band-repeater-compatible", "ca-cert", "tls-cert", "tls-key",
		"bind", "gw-server-ca-cert", "gw-server-tls-cert", "gw-server-tls-key", "gw-server-jwt-secret", "gw-server-bind",
		"redis-url", "postgres-dsn", "db-automigrate", "gw-mqtt-server", "gw-mqtt-username", "gw-mqtt-password",
		"gw-mqtt-ca-cert", "gw-mqtt-tls-cert", "gw-mqtt-tls-key", "nc-server", "nc-ca-cert", "nc-tls-cert",
		"nc-tls-key", "deduplication-delay", "get-downlink-data-delay", "gw-stats-aggregation-intervals",
		"timezone", "gw-create-on-stats", "extra-frequencies", "enable-uplink-channels", "node-session-ttl",
		"log-node-frames", "js-server", "js-ca-cert", "js-tls-cert", "js-tls-key", "installation-margin",
		"rx1-delay", "rx1-dr-offset", "rx2-dr",
	}
	for _, key := range hidden {
		rootCmd.PersistentFlags().MarkHidden(key)
	}

	viper.BindEnv("general.log_level", "LOG_LEVEL")

	// for backwards compatibility
	viper.BindEnv("postgresql.dsn", "POSTGRES_DSN")
	viper.BindEnv("postgresql.automigrate", "DB_AUTOMIGRATE")
	viper.BindEnv("redis.url", "REDIS_URL")
	viper.BindEnv("network_server.net_id", "NET_ID")
	viper.BindEnv("network_server.deduplication_delay", "DEDUPLICATION_DELAY")
	viper.BindEnv("network_server.device_session_ttl", "NODE_SESSION_TTL")
	viper.BindEnv("network_server.get_downlink_data_delay", "GET_DOWNLINK_DATA_DELAY")
	viper.BindEnv("network_server.band.name", "BAND")
	viper.BindEnv("network_server.band.dwell_time_400ms", "BAND_DWELL_TIME_400ms")
	viper.BindEnv("network_server.band.repeater_compatible", "BAND_REPEATER_COMPATIBLE")
	viper.BindEnv("network_server.network_settings.installation_margin", "INSTALLATION_MARGIN")
	viper.BindEnv("network_server.network_settings.rx1_delay", "RX1_DELAY")
	viper.BindEnv("network_server.network_settings.rx1_dr_offset", "RX1_DR_OFFSET")
	viper.BindEnv("network_server.network_settings.rx2_dr", "RX2_DR")
	viper.BindEnv("network_server.network_settings.enabled_uplink_channels_legacy", "ENABLE_UPLINK_CHANNELS")
	viper.BindEnv("network_server.network_settings.extra_channels_legacy", "EXTRA_FREQUENCIES")
	viper.BindEnv("network_server.api.bind", "BIND")
	viper.BindEnv("network_server.api.ca_cert", "CA_CERT")
	viper.BindEnv("network_server.api.tls_cert", "TLS_CERT")
	viper.BindEnv("network_server.api.tls_key", "TLS_KEY")
	viper.BindEnv("network_server.gateway.api.bind", "GW_SERVER_BIND")
	viper.BindEnv("network_server.gateway.api.ca_cert", "GW_SERVER_CA_CERT")
	viper.BindEnv("network_server.gateway.api.tls_cert", "GW_SERVER_TLS_CERT")
	viper.BindEnv("network_server.gateway.api.tls_key", "GW_SERVER_TLS_KEY")
	viper.BindEnv("network_server.gateway.api.jwt_secret", "GW_SERVER_JWT_SECRET")
	viper.BindEnv("network_server.gateway.stats.create_gateway_on_stats", "GW_CREATE_ON_STATS")
	viper.BindEnv("network_server.gateway.stats.timezone", "TIMEZONE")
	viper.BindEnv("network_server.gateway.stats.aggregation_intervals", "GW_STATS_AGGREGATION_INTERVALS")
	viper.BindEnv("network_server.gateway.backend.mqtt.server", "GW_MQTT_SERVER")
	viper.BindEnv("network_server.gateway.backend.mqtt.username", "GW_MQTT_USERNAME")
	viper.BindEnv("network_server.gateway.backend.mqtt.password", "GW_MQTT_PASSWORD")
	viper.BindEnv("network_server.gateway.backend.mqtt.ca_cert", "GW_MQTT_CA_CERT")
	viper.BindEnv("network_server.gateway.backend.mqtt.tls_cert", "GW_MQTT_TLS_CERT")
	viper.BindEnv("network_server.gateway.backend.mqtt.tls_key", "GW_MQTT_TLS_KEY")
	viper.BindEnv("join_server.default.server", "JS_SERVER")
	viper.BindEnv("join_server.default.ca_cert", "JS_CA_CERT")
	viper.BindEnv("join_server.default.tls_cert", "JS_TLS_CERT")
	viper.BindEnv("join_server.default.tls_key", "JS_TLS_KEY")
	viper.BindEnv("network_controller.server", "NC_SERVER")
	viper.BindEnv("network_controller.ca_cert", "NC_CA_CERT")
	viper.BindEnv("network_controller.tls_cert", "NC_TLS_CERT")
	viper.BindEnv("network_controller.tls_key", "NC_TLS_KEY")

	viper.BindPFlag("general.log_level", rootCmd.PersistentFlags().Lookup("log-level"))

	// for backwards compatibility
	viper.BindPFlag("postgresql.dsn", rootCmd.PersistentFlags().Lookup("postgres-dsn"))
	viper.BindPFlag("postgresql.automigrate", rootCmd.PersistentFlags().Lookup("db-automigrate"))
	viper.BindPFlag("redis.url", rootCmd.PersistentFlags().Lookup("redis-url"))
	viper.BindPFlag("network_server.net_id", rootCmd.PersistentFlags().Lookup("net-id"))
	viper.BindPFlag("network_server.deduplication_delay", rootCmd.PersistentFlags().Lookup("deduplication-delay"))
	viper.BindPFlag("network_server.device_session_ttl", rootCmd.PersistentFlags().Lookup("node-session-ttl"))
	viper.BindPFlag("network_server.get_downlink_data_delay", rootCmd.PersistentFlags().Lookup("get-downlink-data-delay"))
	viper.BindPFlag("network_server.band.name", rootCmd.PersistentFlags().Lookup("band"))
	viper.BindPFlag("network_server.band.dwell_time_400ms", rootCmd.PersistentFlags().Lookup("band-dwell-time-400ms"))
	viper.BindPFlag("network_server.band.repeater_compatible", rootCmd.PersistentFlags().Lookup("band-repeater-compatible"))
	viper.BindPFlag("network_server.network_settings.installation_margin", rootCmd.PersistentFlags().Lookup("installation-margin"))
	viper.BindPFlag("network_server.network_settings.rx1_delay", rootCmd.PersistentFlags().Lookup("rx1-delay"))
	viper.BindPFlag("network_server.network_settings.rx1_dr_offset", rootCmd.PersistentFlags().Lookup("rx1-dr-offset"))
	viper.BindPFlag("network_server.network_settings.rx2_dr", rootCmd.PersistentFlags().Lookup("rx2-dr"))
	viper.BindPFlag("network_server.network_settings.enabled_uplink_channels_legacy", rootCmd.PersistentFlags().Lookup("enable-uplink-channels"))
	viper.BindPFlag("network_server.api.bind", rootCmd.PersistentFlags().Lookup("bind"))
	viper.BindPFlag("network_server.api.ca_cert", rootCmd.PersistentFlags().Lookup("ca-cert"))
	viper.BindPFlag("network_server.api.tls_cert", rootCmd.PersistentFlags().Lookup("tls-cert"))
	viper.BindPFlag("network_server.api.tls_key", rootCmd.PersistentFlags().Lookup("tls-key"))
	viper.BindPFlag("network_server.gateway.api.bind", rootCmd.PersistentFlags().Lookup("gw-server-bind"))
	viper.BindPFlag("network_server.gateway.api.ca_cert", rootCmd.PersistentFlags().Lookup("gw-server-ca-cert"))
	viper.BindPFlag("network_server.gateway.api.tls_cert", rootCmd.PersistentFlags().Lookup("gw-server-tls-cert"))
	viper.BindPFlag("network_server.gateway.api.tls_key", rootCmd.PersistentFlags().Lookup("gw-server-tls-key"))
	viper.BindPFlag("network_server.gateway.api.jwt_secret", rootCmd.PersistentFlags().Lookup("gw-server-jwt-secret"))
	viper.BindPFlag("network_server.gateway.stats.create_gateway_on_stats", rootCmd.PersistentFlags().Lookup("gw-create-on-stats"))
	viper.BindPFlag("network_server.gateway.stats.timezone", rootCmd.PersistentFlags().Lookup("timezone"))
	viper.BindPFlag("network_server.gateway.stats.aggregation_intervals", rootCmd.PersistentFlags().Lookup("gw-stats-aggregation-intervals"))
	viper.BindPFlag("network_server.gateway.backend.mqtt.server", rootCmd.PersistentFlags().Lookup("gw-mqtt-server"))
	viper.BindPFlag("network_server.gateway.backend.mqtt.username", rootCmd.PersistentFlags().Lookup("gw-mqtt-username"))
	viper.BindPFlag("network_server.gateway.backend.mqtt.password", rootCmd.PersistentFlags().Lookup("gw-mqtt-password"))
	viper.BindPFlag("network_server.gateway.backend.mqtt.ca_cert", rootCmd.PersistentFlags().Lookup("gw-mqtt-ca-cert"))
	viper.BindPFlag("network_server.gateway.backend.mqtt.tls_cert", rootCmd.PersistentFlags().Lookup("gw-mqtt-tls-cert"))
	viper.BindPFlag("network_server.gateway.backend.mqtt.tls_key", rootCmd.PersistentFlags().Lookup("gw-mqtt-tls-key"))
	viper.BindPFlag("join_server.default.server", rootCmd.PersistentFlags().Lookup("js-server"))
	viper.BindPFlag("join_server.default.ca_cert", rootCmd.PersistentFlags().Lookup("js-ca-cert"))
	viper.BindPFlag("join_server.default.tls_cert", rootCmd.PersistentFlags().Lookup("js-tls-cert"))
	viper.BindPFlag("join_server.default.tls_key", rootCmd.PersistentFlags().Lookup("js-tls-key"))
	viper.BindPFlag("network_controller.server", rootCmd.PersistentFlags().Lookup("nc-server"))
	viper.BindPFlag("network_controller.ca_cert", rootCmd.PersistentFlags().Lookup("nc-ca-cert"))
	viper.BindPFlag("network_controller.tls_cert", rootCmd.PersistentFlags().Lookup("nc-tls-cert"))
	viper.BindPFlag("network_controller.tls_key", rootCmd.PersistentFlags().Lookup("nc-tls-key"))

	// default values
	viper.SetDefault("network_server.gateway.backend.mqtt.uplink_topic_template", "gateway/{{ .MAC }}/rx")
	viper.SetDefault("network_server.gateway.backend.mqtt.downlink_topic_template", "gateway/{{ .MAC }}/tx")
	viper.SetDefault("network_server.gateway.backend.mqtt.stats_topic_template", "gateway/{{ .MAC }}/stats")
	viper.SetDefault("network_server.gateway.backend.mqtt.ack_topic_template", "gateway/{{ .MAC }}/ack")

	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(configCmd)
}

// Execute executes the root command.
func Execute(v string) {
	version = v
	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}

func initConfig() {
	if cfgFile != "" {
		b, err := ioutil.ReadFile(cfgFile)
		if err != nil {
			log.WithError(err).WithField("config", cfgFile).Fatal("error loading config file")
		}
		viper.SetConfigType("toml")
		if err := viper.ReadConfig(bytes.NewBuffer(b)); err != nil {
			log.WithError(err).WithField("config", cfgFile).Fatal("error loading config file")
		}
	} else {
		viper.SetConfigName("loraserver")
		viper.AddConfigPath(".")
		viper.AddConfigPath("$HOME/.config/loraserver")
		viper.AddConfigPath("/etc/loraserver")
		if err := viper.ReadInConfig(); err != nil {
			switch err.(type) {
			case viper.ConfigFileNotFoundError:
				log.Warning("Deprecation warning! no configuration file found, falling back on environment variables. Update your configuration, see: https://docs.loraserver.io/loraserver/install/config/")
			default:
				log.WithError(err).Fatal("read configuration file error")
			}
		}
	}

	if err := viper.Unmarshal(&config.C); err != nil {
		log.WithError(err).Fatal("unmarshal config error")
	}

	if err := config.C.NetworkServer.NetID.UnmarshalText([]byte(config.C.NetworkServer.NetIDString)); err != nil {
		log.WithError(err).Fatal("decode net_id error")
	}

	if len(config.C.NetworkServer.NetworkSettings.ExtraChannels) == 0 && len(config.C.NetworkServer.NetworkSettings.ExtraChannelsLegacy) != 0 {
		for _, freq := range config.C.NetworkServer.NetworkSettings.ExtraChannelsLegacy {
			config.C.NetworkServer.NetworkSettings.ExtraChannels = append(config.C.NetworkServer.NetworkSettings.ExtraChannels, struct {
				Frequency int
				MinDR     int `mapstructure:"min_dr"`
				MaxDR     int `mapstructure:"max_dr"`
			}{
				Frequency: freq,
				MinDR:     0,
				MaxDR:     5,
			})
		}
	}

	if config.C.NetworkServer.NetworkSettings.EnabledUplinkChannelsLegacy != "" && len(config.C.NetworkServer.NetworkSettings.EnabledUplinkChannels) == 0 {
		blocks := strings.Split(config.C.NetworkServer.NetworkSettings.EnabledUplinkChannelsLegacy, ",")
		for _, block := range blocks {
			block = strings.Trim(block, " ")
			var start, end int
			if _, err := fmt.Sscanf(block, "%d-%d", &start, &end); err != nil {
				if _, err := fmt.Sscanf(block, "%d", &start); err != nil {
					log.WithError(err).Fatal("parse channel range error")
				}
				end = start
			}

			for ; start <= end; start++ {
				config.C.NetworkServer.NetworkSettings.EnabledUplinkChannels = append(config.C.NetworkServer.NetworkSettings.EnabledUplinkChannels, start)
			}
		}
	}
}
