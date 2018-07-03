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
	string(band.RU_864_870),
	string(band.US_902_928),
}

var rootCmd = &cobra.Command{
	Use:   "loraserver",
	Short: "LoRa Server network-server",
	Long: `LoRa Server is an open-source network-server, part of the LoRa Server project
	> documentation & support: https://www.loraserver.io/loraserver/
	> source & copyright information: https://github.com/brocaar/loraserver/`,
	RunE: run,
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "path to configuration file (optional)")
	rootCmd.PersistentFlags().Int("log-level", 4, "debug=5, info=4, error=2, fatal=1, panic=0")

	viper.BindPFlag("general.log_level", rootCmd.PersistentFlags().Lookup("log-level"))

	// default values
	viper.SetDefault("network_server.net_id", "000000")
	viper.SetDefault("network_server.band.name", "EU_863_870")
	viper.SetDefault("network_server.api.bind", "0.0.0.0:8000")
	viper.SetDefault("redis.url", "redis://localhost:6379")
	viper.SetDefault("postgresql.dsn", "postgres://localhost/loraserver_ns?sslmode=disable")
	viper.SetDefault("postgresql.automigrate", true)
	viper.SetDefault("network_server.gateway.backend.mqtt.server", "tcp://localhost:1883")
	viper.SetDefault("network_server.deduplication_delay", 200*time.Millisecond)
	viper.SetDefault("network_server.get_downlink_data_delay", 100*time.Millisecond)
	viper.SetDefault("network_server.gateway.stats.aggregation_intervals", []string{"minute", "hour", "day"})
	viper.SetDefault("network_server.gateway.stats.create_gateway_on_stats", true)
	viper.SetDefault("network_server.device_session_ttl", time.Hour*24*31)
	viper.SetDefault("join_server.default.server", "http://localhost:8003")
	viper.SetDefault("network_server.network_settings.installation_margin", 10)
	viper.SetDefault("network_server.network_settings.rx1_delay", 1)
	viper.SetDefault("network_server.network_settings.rx2_frequency", -1)
	viper.SetDefault("network_server.network_settings.rx2_dr", -1)
	viper.SetDefault("network_server.network_settings.disable_adr", false)
	viper.SetDefault("network_server.gateway.backend.mqtt.uplink_topic_template", "gateway/+/rx")
	viper.SetDefault("network_server.gateway.backend.mqtt.downlink_topic_template", "gateway/{{ .MAC }}/tx")
	viper.SetDefault("network_server.gateway.backend.mqtt.stats_topic_template", "gateway/+/stats")
	viper.SetDefault("network_server.gateway.backend.mqtt.ack_topic_template", "gateway/+/ack")
	viper.SetDefault("network_server.gateway.backend.mqtt.config_topic_template", "gateway/{{ .MAC }}/config")
	viper.SetDefault("network_server.gateway.backend.mqtt.clean_session", true)

	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(printDSCmd)
}

// Execute executes the root command.
func Execute(v string) {
	version = v
	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}

func initConfig() {
	config.Version = version

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
				log.Warning("No configuration file found, using defaults. See: https://www.loraserver.io/loraserver/install/config/")
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
