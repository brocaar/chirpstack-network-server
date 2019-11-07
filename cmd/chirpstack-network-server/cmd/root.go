package cmd

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/mitchellh/mapstructure"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/brocaar/chirpstack-network-server/internal/config"
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
	Use:   "chirpstack-network-server",
	Short: "ChirpStack Network Server",
	Long: `ChirpStack Network Server is an open-source LoRaWAN Network Server, part of the ChirpStack Network Server stack
	> documentation & support: https://www.chirpstack.io/network-server/
	> source & copyright information: https://github.com/brocaar/chirpstack-network-server/`,
	RunE: run,
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "path to configuration file (optional)")
	rootCmd.PersistentFlags().Int("log-level", 4, "debug=5, info=4, error=2, fatal=1, panic=0")

	viper.BindPFlag("general.log_level", rootCmd.PersistentFlags().Lookup("log-level"))

	// default values
	viper.SetDefault("redis.url", "redis://localhost:6379")
	viper.SetDefault("redis.max_idle", 10)
	viper.SetDefault("redis.idle_timeout", 5*time.Minute)

	viper.SetDefault("postgresql.dsn", "postgres://localhost/chirpstack_ns?sslmode=disable")
	viper.SetDefault("postgresql.automigrate", true)
	viper.SetDefault("postgresql.max_idle_connections", 2)

	viper.SetDefault("network_server.net_id", "000000")
	viper.SetDefault("network_server.band.name", "EU_863_870")
	viper.SetDefault("network_server.band.uplink_max_eirp", -1)
	viper.SetDefault("network_server.api.bind", "0.0.0.0:8000")

	viper.SetDefault("network_server.deduplication_delay", 200*time.Millisecond)
	viper.SetDefault("network_server.get_downlink_data_delay", 100*time.Millisecond)
	viper.SetDefault("network_server.device_session_ttl", time.Hour*24*31)

	viper.SetDefault("network_server.gateway.stats.aggregation_intervals", []string{"minute", "hour", "day"})
	viper.SetDefault("network_server.gateway.stats.create_gateway_on_stats", true)
	viper.SetDefault("network_server.gateway.backend.mqtt.server", "tcp://localhost:1883")

	viper.SetDefault("join_server.default.server", "http://localhost:8003")

	viper.SetDefault("network_server.network_settings.installation_margin", 10)
	viper.SetDefault("network_server.network_settings.rx1_delay", 1)
	viper.SetDefault("network_server.network_settings.rx2_frequency", -1)
	viper.SetDefault("network_server.network_settings.rx2_dr", -1)
	viper.SetDefault("network_server.network_settings.downlink_tx_power", -1)
	viper.SetDefault("network_server.network_settings.disable_adr", false)

	viper.SetDefault("network_server.gateway.backend.type", "mqtt")

	viper.SetDefault("network_server.scheduler.scheduler_interval", 1*time.Second)
	viper.SetDefault("network_server.scheduler.class_c.downlink_lock_duration", 2*time.Second)
	viper.SetDefault("network_server.gateway.backend.mqtt.event_topic", "gateway/+/event/+")
	viper.SetDefault("network_server.gateway.backend.mqtt.command_topic_template", "gateway/{{ .GatewayID }}/command/{{ .CommandType }}")
	viper.SetDefault("network_server.gateway.backend.mqtt.clean_session", true)
	viper.SetDefault("join_server.resolve_domain_suffix", ".joineuis.lora-alliance.org")
	viper.SetDefault("join_server.default.server", "http://localhost:8003")

	viper.SetDefault("network_server.gateway.backend.gcp_pub_sub.uplink_retention_duration", time.Hour*24)

	viper.SetDefault("metrics.timezone", "Local")
	viper.SetDefault("metrics.redis.aggregation_intervals", []string{"MINUTE", "HOUR", "DAY", "MONTH"})
	viper.SetDefault("metrics.redis.minute_aggregation_ttl", time.Hour*2)
	viper.SetDefault("metrics.redis.hour_aggregation_ttl", time.Hour*48)
	viper.SetDefault("metrics.redis.day_aggregation_ttl", time.Hour*24*90)
	viper.SetDefault("metrics.redis.month_aggregation_ttl", time.Hour*24*730)

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
		viper.SetConfigName("chirpstack-network-server")
		viper.AddConfigPath(".")
		viper.AddConfigPath("$HOME/.config/chirpstack-network-server")
		viper.AddConfigPath("/etc/chirpstack-network-server")
		if err := viper.ReadInConfig(); err != nil {
			switch err.(type) {
			case viper.ConfigFileNotFoundError:
				log.Warning("No configuration file found, using defaults. See: https://www.chirpstack.io/network-server/install/config/")
			default:
				log.WithError(err).Fatal("read configuration file error")
			}
		}
	}

	for _, pair := range os.Environ() {
		d := strings.SplitN(pair, "=", 2)
		if strings.Contains(d[0], ".") {
			log.Warning("Using dots in env variable is illegal and deprecated. Please use double underscore `__` for: ", d[0])
			underscoreName := strings.ReplaceAll(d[0], ".", "__")
			// Set only when the underscore version doesn't already exist.
			if _, exists := os.LookupEnv(underscoreName); !exists {
				os.Setenv(underscoreName, d[1])
			}
		}
	}

	viperBindEnvs(config.C)

	viperHooks := mapstructure.ComposeDecodeHookFunc(
		viperDecodeJSONSlice,
		mapstructure.StringToTimeDurationHookFunc(),
		mapstructure.StringToSliceHookFunc(","),
	)

	if err := viper.Unmarshal(&config.C, viper.DecodeHook(viperHooks)); err != nil {
		log.WithError(err).Fatal("unmarshal config error")
	}

	if err := config.C.NetworkServer.NetID.UnmarshalText([]byte(config.C.NetworkServer.NetIDString)); err != nil {
		log.WithError(err).Fatal("decode net_id error")
	}
}

func viperBindEnvs(iface interface{}, parts ...string) {
	ifv := reflect.ValueOf(iface)
	ift := reflect.TypeOf(iface)
	for i := 0; i < ift.NumField(); i++ {
		v := ifv.Field(i)
		t := ift.Field(i)
		tv, ok := t.Tag.Lookup("mapstructure")
		if !ok {
			tv = strings.ToLower(t.Name)
		}
		if tv == "-" {
			continue
		}

		switch v.Kind() {
		case reflect.Struct:
			viperBindEnvs(v.Interface(), append(parts, tv)...)
		default:
			// Bash doesn't allow env variable names with a dot so
			// bind the double underscore version.
			keyDot := strings.Join(append(parts, tv), ".")
			keyUnderscore := strings.Join(append(parts, tv), "__")
			viper.BindEnv(keyDot, strings.ToUpper(keyUnderscore))
		}
	}
}

func viperDecodeJSONSlice(rf reflect.Kind, rt reflect.Kind, data interface{}) (interface{}, error) {
	// input must be a string and destination must be a slice
	if rf != reflect.String || rt != reflect.Slice {
		return data, nil
	}

	raw := data.(string)

	// this decoder expects a JSON list
	if !strings.HasPrefix(raw, "[") || !strings.HasSuffix(raw, "]") {
		return data, nil
	}

	var out []map[string]interface{}
	err := json.Unmarshal([]byte(raw), &out)

	return out, err
}
