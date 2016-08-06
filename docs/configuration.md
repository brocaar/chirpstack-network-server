# Configuration

All configuration is either done by command-line arguments or environment variables, or
a mix of both. Execute ``./loraserver --help`` to see all available configuration
options for your version.

#### --net-id / NET_ID

This sets the ``NetID`` of your LoRaWAN network. Taken from the LoRaWAN specifications:

> The format of the NetID is as follows: The seven LSB of the NetID are called NwkID and
> match the seven MSB of the short address of an end-device as described before.
> Neighboring or overlapping networks must have different NwkIDs. The remaining 17 MSB
> can be freely chosen by the network operator.

The value needs to be [HEX](https://en.wikipedia.org/wiki/Hexadecimal) encoded, e.g. ``010203``.

#### --band / BAND

This sets the ISM band specific configuration to use. Executing ``./loraserver --help``
will show you all the available ISM bands available.

#### --http-bind / HTTP_BIND

This sets the ``IP:PORT`` on which the http server
(web-interface and REST API) will bind.

#### --http-tls-cert / HTTP_TLS_CERT

Path to the TLS certificate for the http server.

#### --http-tls-key / HTTP_TLS_KEY

Path to the TLS key for the http server.

#### --grpc-bind / GRPC_BIND

This sets the ``IP:PORT`` on which the gRPC server
will bind.

#### --grpc-tls-cert / GRPC_TLS_CERT

Path to the TLS certificate for the gRPC server.

#### --grpc-tls-key / GRPC_TLS_KEY

Path to the TLS key for the gRPC server.

#### --postgres-dsn / POSTGRES_DSN

This sets the PostgreSQL data-source name.
See [Connection String Parameters](https://godoc.org/github.com/lib/pq#hdr-Connection_String_Parameters)
for all available options.

#### --redis-url / REDIS_URL

This sets the Redis URL, see [https://www.iana.org/assignments/uri-schemes/prov/redis](https://www.iana.org/assignments/uri-schemes/prov/redis) for all available options.

#### --db-automigrate / DB_AUTOMIGRATE

Given that you already created the PostgreSQL database, this will apply all
(forward) database migrations. If you prefer to apply these migrations manually,
see the [migrations](https://github.com/brocaar/loraserver/tree/master/migrations)
folder in the source repository for the raw SQL.

#### --gw-mqtt-server / GW_MQTT_SERVER

This sets the MQTT server to connect the gateway backend to.

#### --gw-mqtt-username / GW_MQTT_USERNAME

This sets the MQTT username for the gateway backend.

#### --gw-mqtt-password / GW_MQTT_PASSWORD

This sets the MQTT password for the gateway backend.

#### --app-mqtt-server / APP_MQTT_SERVER

This sets the MQTT server to connect the application backend to.

#### --app-mqtt-username / APP_MQTT_USERNAME

This sets the MQTT username for the application backend.

#### --app-mqtt-password / APP_MQTT_PASSWORD

This sets the MQTT password for the application backend.

#### --controller-mqtt-server / CONTROLLER_MQTT_SERVER

This sets the MQTT server to connect the network-controller backend to.

#### --controller-mqtt-username / CONTROLLER_MQTT_USERNAME

This sets the MQTT username for the network-controller backend.

#### --controller-mqtt-password / CONTROLLER_MQTT_PASSWORD

This sets the MQTT password for the network-controller backend.

