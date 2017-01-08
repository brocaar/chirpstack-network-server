# Configuration

To list all configuration options, start `loraserver` with the `--help`
flag. This will display:

```
GLOBAL OPTIONS:
   --net-id value                   network identifier (NetID, 3 bytes) encoded as HEX (e.g. 010203) [$NET_ID]
   --band value                     ism band configuration to use (options: AS_923, AU_915_928, CN_470_510, CN_779_787, EU_433, EU_863_870, KR_920_923, RU_864_869, US_902_928) [$BAND]
   --band-dwell-time-400ms          band configuration takes 400ms dwell-time into account [$BAND_DWELL_TIME_400ms]
   --band-repeater-compatible       band configuration takes repeater encapsulation layer into account [$BAND_REPEATER_COMPATIBLE]
   --ca-cert value                  ca certificate used by the api server (optional) [$CA_CERT]
   --tls-cert value                 tls certificate used by the api server (optional) [$TLS_CERT]
   --tls-key value                  tls key used by the api server (optional) [$TLS_KEY]
   --bind value                     ip:port to bind the api server (default: "0.0.0.0:8000") [$BIND]
   --redis-url value                redis url (e.g. redis://user:password@hostname:port/0) (default: "redis://localhost:6379") [$REDIS_URL]
   --gw-mqtt-server value           mqtt broker server used by the gateway backend (e.g. scheme://host:port where scheme is tcp, ssl or ws) (default: "tcp://localhost:1883") [$GW_MQTT_SERVER]
   --gw-mqtt-username value         mqtt username used by the gateway backend (optional) [$GW_MQTT_USERNAME]
   --gw-mqtt-password value         mqtt password used by the gateway backend (optional) [$GW_MQTT_PASSWORD]
   --as-server value                hostname:port of the application-server api server (optional) (default: "127.0.0.1:8001") [$AS_SERVER]
   --as-ca-cert value               ca certificate used by the application-server client (optional) [$AS_CA_CERT]
   --as-tls-cert value              tls certificate used by the application-server client (optional) [$AS_TLS_CERT]
   --as-tls-key value               tls key used by the application-server client (optional) [$AS_TLS_KEY]
   --nc-server value                hostname:port of the network-controller api server (optional) [$NC_SERVER]
   --nc-ca-cert value               ca certificate used by the network-controller client (optional) [$NC_CA_CERT]
   --nc-tls-cert value              tls certificate used by the network-controller client (optional) [$NC_TLS_CERT]
   --nc-tls-key value               tls key used by the network-controller client (optional) [$NC_TLS_KEY]
   --deduplication-delay value      time to wait for uplink de-duplication (default: 200ms) [$DEDUPLICATION_DELAY]
   --get-downlink-data-delay value  delay between uplink delivery to the app server and getting the downlink data from the app server (if any) (default: 100ms) [$GET_DOWNLINK_DATA_DELAY]
   --help, -h                       show help
   --version, -v                    print the version
```

Both cli arguments and environment-variables can be used to pass configuration
options.

## NetID

Taken from the LoRaWAN specifications:

> The format of the NetID is as follows: The seven LSB of the NetID are called NwkID and
> match the seven MSB of the short address of an end-device as described before.
> Neighboring or overlapping networks must have different NwkIDs. The remaining 17 MSB
> can be freely chosen by the network operator.

The value needs to be [HEX](https://en.wikipedia.org/wiki/Hexadecimal) encoded, e.g. ``010203``.

## Band

It is important to start `loraserver` with the correct band, as this defines
the frequencies used. Make sure these frequencies match the frequencies as
configured in your gateways.

## Dwell time

Some band configurations define the max payload size for both dwell-time
limitation enabled as disabled (e.g. AS 923). In this case the
`--band-dwell-time-400ms` flag must be set to enforce the max payload size
given the dwell-time limitation. For band configuration where the dwell-time is
always enforced, setting this flag is not required.

## Repeater compatibility

Most band configurations define the max payload size for both an optional
repeater encapsulation layer as for setups where a repeater will never
be used. The latter case increases the max payload size for some data-rates.
In case a repeater might used, set the `--band-repeater-compatible` flag.

## Redis connection string

For more information about the Redis URL format, see:
[https://www.iana.org/assignments/uri-schemes/prov/redis](https://www.iana.org/assignments/uri-schemes/prov/redis).
