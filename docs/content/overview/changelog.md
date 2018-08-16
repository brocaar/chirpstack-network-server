---
title: Changelog
menu:
    main:
        parent: overview
        weight: 3
---
# Changelog

## v2.1.0 (in development)

### Features

#### Updated data-format between LoRa Server and LoRa Gateway Bridge

Note that this is a backwards compatible change as LoRa Server is able to
automatically detect the used serizalization format based on the data sent by
the LoRa Gateway Bridge.

##### Protocol Buffer data serialization

This adds support for the [Protocol Buffers](https://developers.google.com/protocol-buffers/)
data serialization introduced by LoRa Gateway Bridge v2.5.0 to save on
bandwidth between the LoRa Gateway Bridge and the MQTT.

##### New JSON format

The new JSON structure re-uses the messages defined for
[Protocol Buffers](https://developers.google.com/protocol-buffers/docs/proto3#json)
based serialization.

## v2.0.2

### Bugfixes

* Fix flush device- and service-profile cache on clean database. ([#345](https://github.com/brocaar/loraserver/issues/345))

## v2.0.1

### Bugfixes

* Use `gofrs/uuid` UUID library as `satori/go.uuid` is not truly random. ([#342](https://github.com/brocaar/loraserver/pull/342))
* Flush device- and service-profile cache when migrating from v1 to v2. ([lora-app-server#254](https://github.com/brocaar/lora-app-server/issues/254))
* Set `board` and `antenna` on downlink. ([#341](https://github.com/brocaar/loraserver/pull/341))

## v2.0.0

### Upgrade nodes

Before upgrading to v2, first make sure you have the latest v1 installed and running
(including LoRa App Server). As always, it is recommended to make a backup
first :-)

### Features

* LoRaWAN 1.1 support!
* Support for signaling received (encrypted) AppSKey from join-server to
  application-server on security context change.
* Support for Key Encryption Keys, used for handling encrypted keys from the
  join-server.

### Changes

* LoRa Server calls the `SetDeviceStatus` API method of LoRa App Server
  when it receives a `DevStatusAns` mac-command.
* Device-sessions are stored using Protobuf encoding in Redis
  (more compact storage).
* Cleanup of gRPC API methods and arguments to follow the Protobuf style-guide
  and to make message re-usable. When you're integrating directly with the
  LoRa Server gRPC API, then you must update your API client as these changes are
  backwards incompatible!
* Config option added to globally disable ADR.
* Config option added to override default downlink tx power.

## v1.0.1

### Features

* Config option added to disable mac-commands (for testing).

## v1.0.0

This marks the first stable release! 

### Upgrade notes

* First make sure you have v0.26.3 installed and running (including LoRa App Server v21.1).
* Then ugrade to v1.0.0.

See [Downloads](https://www.loraserver.io/loraserver/overview/downloads/)
for pre-compiled binaries or instructions how to setup the Debian / Ubuntu
repository for v1.x.

### Changes

* Code to remain backwards compatible with environment-variable based
  configuration has been removed.
* Code to migrate node- to device-sessions has been removed.
* Code to migrate channel-configuration to gateway-profiles has been removed.
* Old unused tables (kept for upgrade migration code) have been removed from db.

## 0.26.3

**Bugfixes:**

* Fixes an "index out of range" issue when removing conflicting mac-commands. ([#323](https://github.com/brocaar/loraserver/issues/323))

## 0.26.2

**Bugfixes:**

* On decreasing the TXPower index to `0` (nACKed by Microchip RN devices), LoRa Server would keep sending LinkADRReq mac-commands.
  On a TXPower index `0` nACK, LoRa Server will now set the min TXPower index to `1` as a workaround.
* On deleting a device, the device-session is now flushed.
* `NewChannelReq` and `LinkADRReq` mac-commands were sometimes sent together, causing the new channel to be disabled by the `LinkADRReq` channel-mask (not aware yet about the new channel).

**Improvements:**

* `NbTrans` is set to `1` on activation, to avoid transitioning from `0` to `1` (effectively the same).

## 0.26.1

**Improvements:**

* `HandleUplinkData` API call to the application-server is now handled async.
* Skip frame-counter check can now be set per device (so it can be used for OTAA devices).

**Bugfixes:**

* `storage.ErrAlreadyExists` was not mapped to the correct gRPC API error.

## 0.26.0

**Features:**

* (Gateway) channel-configuration has been refactored into gateway-profiles and
  configuration updates are now sent over MQTT to the gateway.
  * This requires [LoRa Gateway Bridge](https://www.loraserver.io/lora-gateway-bridge/) 2.4.0 or up.
  * This requires [LoRa App Server](https://www.loraserver.io/lora-app-server/) 0.20.0 or up.
  * This deprecates the [LoRa Channel Manager](https://www.loraserver.io/lora-channel-manager/) service.
  * This removes the `Gateway` gRPC service (which was running by default on port `8002`).
  * This removes the channel-configuration related gRPC methods from the `NetworkServer` gRPC service.
  * This adds gateway-profile related gRPC methods to the `NetworkServer` gRPC service.

* FSK support when permitted by the LoRaWAN ISM band.
  * Note that the ADR engine will only use the data-rates of the pre-defined multi data-rate channels.

**Bugfixes:**

* Fix leaking Redis connections on pubsub subscriber ([#313](https://github.com/brocaar/loraserver/issues/313).

**Upgrade notes:**

In order to automatically migrate the existing channel-configuration into the
new gateway-profiles, first upgrade LoRa Server and restart it. After upgrading
LoRa App Server and restarting it, all channel-configurations will be migrated
and associated to the gateways. As always, it is advised to first make a backup
of your (PostgreSQL) database.

## 0.25.1

**Features:**

* Add `RU_864_870` as configuration option (thanks [@belovictor](https://github.com/belovictor))

**Improvements:**

* Expose the following MQTT options for the MQTT gateway backend:
  * QoS (quality of service)
  * Client ID
  * Clean session on connect
* Add `GetVersion` API method returning the LoRa Server version + configured region.
* Refactor `lorawan/band` package with support for max payload-size per
  LoRaWAN mac version and Regional Parameters revision.
  * This avoids packetloss in case a device does not implement the latest
    LoRaWAN Regional Parameters revision and the max payload-size values
    have been updated.

**Bugfixes:**

* MQTT topics were hardcoded in configuration file template, this has been fixed.
* Fix `network_contoller` -> `network_controller` typo ([#302](https://github.com/brocaar/loraserver/issues/302))
* Fix typo in pubsub key (resulting in ugly Redis keys) ([#296](https://github.com/brocaar/loraserver/pull/296))

## 0.25.0

**Features:**

* Class-B support! See [Device classes](https://docs.loraserver.io/loraserver/features/device-classes/)
  for more information on Class-B.
  * Class-B configuration can be found under the `network_server.network_settings.class_b`
   [configuration](https://docs.loraserver.io/loraserver/install/config/) section.
  * **Note:** This requires [LoRa Gateway Bridge](https://docs.loraserver.io/lora-gateway-bridge/overview/)
    2.2.0 or up.

* Extended support for extra channel configuration using the NewChannelReq mac-command.
  This makes it possible to:
  * Configure up to 16 channels in total (if supported by the LoRaWAN region).
  * Configure the min / max data-rate range for these extra channels.

* Implement RXParamSetup mac-command. After a configuration file change,
  LoRa Server will push the RX2 frequency, RX2 data-rate and RX1 data-rate
  offset for activated devices.

* Implement RXTimingSetup mac-command. After a configuration file change,
  LoRa Server will push the RX delay for activated devices.

## 0.24.3

**Bugfixes:**

* The uplink, stats and ack topic contained invalid defaults.

## 0.24.2

**Improvements:**

* MQTT topics are now configurable through the configuration file.
  See [Configuration](https://docs.loraserver.io/loraserver/install/config/).

* Internal cleanup of mac-command handling.
  * When issuing mac-commands, they are directly added to the downlink
    context instead of being stored in Redis and then retrieved.
  * For API consistency, the gRPC method
  `EnqueueDownlinkMACCommand` has been renamed to `CreateMACCommandQueueItem`.

**Bugfixes:**

* Fix typo in `create_gateway_on_stats` config mapping. (thanks [@mkiiskila](https://github.com/mkiiskila), [#295](https://github.com/brocaar/loraserver/pull/295))

## 0.24.1

**Bugfixes:**

* Fix basing tx-power value on wrong SNR value (thanks [@x0y1z2](https://github.com/x0y1z2), [#293](https://github.com/brocaar/loraserver/issues/293))

## 0.24.0

**Features:**

* LoRa Server uses a new configuration file format.
  See [configuration](https://docs.loraserver.io/loraserver/install/config/) for more information.
* `StreamFrameLogsForGateway` API method has been added to stream frames for a given gateway MAC.
* `StreamFrameLogsForDevice` API method has been added to stream frames for a given DevEUI.
* Support MQTT client certificate authentication ([#284](https://github.com/brocaar/loraserver/pull/284)).

**Changes:**

* `GetFrameLogsForDevEUI` API method has been removed. The `frame_log` table
  will be removed from the database in the next release!

**Upgrade notes:**

When upgrading using the `.deb` package / using `apt` or `apt-get`, your
configuration will be automatically migrated for you. In any other case,
please see [configuration](https://docs.loraserver.io/loraserver/install/config/).

## 0.23.3

**Improvements:**

* Device-status (battery and link margin) returns `256` as value when battery
  and / or margin status is (yet) not available.
* Extra logging has been added:
  * gRPC API calls (to the gRPC server and by the gRPC clients) are logged
    as `info`
  * Executed SQL queries are logged as `debug`
* LoRa Server will wait 2 seconds between scheduling Class-C downlink
  transmissions to the same device, to avoid that sequential Class-C downlink
  transmissions collide (in case of running a cluster of LoRa Server instances).

**Internal changes:**

* The project moved to using [dep](https://github.com/golang/dep) as vendoring
  tool. In case you run into compiling issues, remove the `vendor` folder
  (which is not part of the repository anymore) and run `make requirements`.

## 0.23.2

**Features:**

* Implement client certificate validation for incoming API connections.
* Implement client certificate for API connections to LoRa App Server.

This removes the following CLI options:

* `--as-ca-cert`
* `--as-tls-cert`
* `--as-tls-key`

See for more information:

* [LoRa Server configuration](https://docs.loraserver.io/loraserver/install/config/)
* [LoRa App Server configuration](https://docs.loraserver.io/lora-app-server/install/config/)
* [LoRa App Server network-server management](https://docs.loraserver.io/lora-app-server/use/network-servers/)
* [https://github.com/brocaar/loraserver-certificates](https://github.com/brocaar/loraserver-certificates)

## 0.23.1

**Features:**

* LoRa Server sets a random token for each downlink transmission.

**Bugfixes:**

* Add missing `nil` pointer check for `Time`
  ([#280](https://github.com/brocaar/loraserver/issues/280))
* Fix increase of NbTrans (re-transmissions) in case of early packetloss.
* Fix decreasing NbTrans (this only happened in case of data-rate or TX
  power change).

## 0.23.0

**Features:**

* The management of the downlink device-queue has moved to LoRa Server.
  Based on the device-class (A or C and in the future B), LoRa Server will
  decide how to schedule the downlink transmission.
* LoRa Server sends nACK on Class-C confirmed downlink timeout
  (can be set in the device-profile) to the application-server.

**Changes:**

Working towards a consistent and stable API, the following API changes have
been made:

Application-server API

* `HandleDataDownACK` renamed to `HandleDownlinkACK`
* `HandleDataUp` renamed to `HandleUplinkData`
* `HandleProprietaryUp` renamed to `HandleProprietaryUplink`
* `GetDataDown` has been removed (as LoRa Server is now responsible for the
  downlink queue)

Network-server API

* Added
  * `CreateDeviceQueueItem`
  * `FlushDeviceQueueForDevEUI`
  * `GetDeviceQueueItemsForDevEUI`

* Removed
  * `SendDownlinkData`

**Note:** these changes require LoRa App Server 0.15.0 or higher.

## 0.22.1

**Features:**

* Service-profile `DevStatusReqFreq` option has been implemented
  (periodical device-status request).

**Bugfixes:**

* RX2 data-rate was set incorrectly, causing *maximum payload size exceeded*
  errors. (thanks [@maxximal](https://github.com/maxximal))

**Cleanup:**

* Prefix de-duplication Redis keys with `lora:ns:` instead of `loraserver:`
  for consistency.

## 0.22.0

**Note:** this release brings many changes! Make sure (as always) to make a
backup of your PostgreSQL and Redis database before upgrading.

**Changes:**

* Data-model refactor to implement service-profile, device-profile and
  routing-profile storage as defined in the
  [LoRaWAN backend interfaces](https://www.lora-alliance.org/lorawan-for-developers).

* LoRa Server now uses the LoRa App Server Join-Server API as specified by the
  LoRaWAN backend interfaces specification (currently hard-configured endpoint).

* Adaptive data-rate configuration is now globally configured by LoRa Server.
  See [configuration](https://docs.loraserver.io/loraserver/install/config/).

* OTAA RX configuration (RX1 delay, RX1 data-rate offset and RX2 dat-rate) is
  now globally configured by LoRa Server.
  See [configuration](https://docs.loraserver.io/loraserver/install/config/).

**API changes:**

* Service-profile CRUD methods added
* Device-profile CRUD methods added
* Routing-profile CRUD methods added
* Device CRUD methods added
* Device (de)activation methods added
* Node-session related methods have been removed
* `EnqueueDataDownMACCommand` renamed to `EnqueueDownlinkMACCommand`
* `PushDataDown` renamed to `SendDownlinkData`

### How to upgrade

**Note:** this release brings many changes! Make sure (as always) to make a
backup of your PostgreSQL and Redis database before upgrading.

**Note:** When LoRa App Server is running on a different server than LoRa Server,
make sure to set the `--js-server` / `JS_SERVER` (default `localhost:8003`).

This release depends on the latest LoRa App Server release (0.14). Upgrade
LoRa Server first, then proceed with upgrading LoRa App Server. See also the
[LoRa App Server changelog](https://docs.loraserver.io/lora-app-server/overview/changelog/).

## 0.21.0

**Features:**

* Implement sending and receiving 'Proprietary' LoRaWAN message type.
  LoRa Server now implements an API method for sending downlink LoRaWAN frames
  using the 'Proprietary' message-type. 'Proprietary' uplink messages will be
  de-duplicated by LoRa Server, before being forwarded to LoRa App Server.

* ARM64 binaries are now provided.

**Internal improvements:**

* Various parts of the codebase have been cleaned up in preparation for the
  upcoming LoRaWAN 1.1 changes.

## 0.20.1

**Features:**

* Add support for `IN_865_867` ISM band.

**Bugfixes:**

* Remove gateway location and altitude 'nullable' option in the database.
  This removes some complexity and fixes a nil pointer issue when compiled
  using Go < 1.8 ([#210](https://github.com/brocaar/loraserver/issues/210)).

* Update `AU_915_928` data-rates according to the LoRaWAN Regional Parameters
  1.0.2 specification.

* Better handling of ADR and TXPower nACK. In case of a nACK, LoRa Server will
  set the max supported DR / TXPower to the requested value - 1.

* The ADR engine sets the stored node TXPower to `0` when the node uses an
  "unexpected" data-rate for uplink. This is to deal with nodes that are
  regaining connectivity by lowering the data-rate and setting the TXPower
  back to `0`.

## 0.20.0

**Features:**

* LoRa Server now offers the possiblity to configure channel-plans which can
  be assigned to gateways. It exposes an API (by default on port `8002`) which
  can be used by [LoRa Gateway Config](https://docs.loraserver.io/lora-gateway-config/).
  An UI for channel-configurations is provided by [LoRa App Server](https://docs.loraserver.io/lora-app-server/)
  version 0.11.0+.

**Note:** Before upgrading, make sure to configure the `--gw-server-jwt-secret`
/ `GW_SERVER_JWT_SECRET` configuration flag!

## 0.19.2

**Improvements:**

* The ADR engine has been updated together with the `lorawan/band` package
  which now implements the LoRaWAN Regional Parameters 1.0.2 specification.

**Removed:**

* Removed `RU_864_869` band. This band is not officially defined by the
  LoRa Alliance.

**Note:** To deal with nodes implementing the Regional Parameters 1.0 **and**
nodes implementing 1.0.2, the ADR engine will now only increase the TX power
index of the node by one step. This is to avoid that the ADR engine would
switch a node to an unsupported TX power index.

## 0.19.1

**Improvements:**

* `--gw-mqtt-ca-cert` / `GW_MQTT_CA_CERT` configuration flag was added to
  specify an optional CA certificate
  (thanks [@siscia](https://github.com/siscia)).

**Bugfixes:**

* MQTT client library update which fixes an issue where during a failed
  re-connect the protocol version would be downgraded
  ([paho.mqtt.golang#116](https://github.com/eclipse/paho.mqtt.golang/issues/116)).

## 0.19.0

**Changes:**

* `NetworkServer.EnqueueDataDownMACCommand` has been refactored in order to
  support sending of mac-command blocks (guaranteed to be sent as a single
  frame). Acknowledgements on mac-commands sent throught the API will be
  sent to the `NetworkController.HandleDataUpMACCommandRequest` API method.
* `NetworkController.HandleDataUpMACCommandRequest` has been updated to handle
  blocks of mac-commands.
* `NetworkController.HandleError` method has been removed.

**Note:** In case you are using the gRPC API interface of LoRa Server,
this might be a breaking change because of the above changes to the APi methods.
For a code-example, please see the [Network-controller](https://docs.loraserver.io/loraserver/integrate/network-controller/)
documentation.

**Bugfixes:**

* Updated vendored libraries to include MQTT reconnect issue
  ([eclipse/paho.mqtt.golang#96](https://github.com/eclipse/paho.mqtt.golang/issues/96)).

## 0.18.0

**Features:**

* Add configuration option to log all uplink / downlink frames into a database
  (`--log-node-frames` / `LOG_NODE_FRAMES`).

## 0.17.2

**Bugfixes:**

* Do not reset downlink frame-counter in case of relax frame-counter mode as
  this would also reset the downlink counter on a re-transmit.

## 0.17.1

**Features:**

* TTL of node-sessions in Redis is now configurable through
  `--node-session-ttl` / `NODE_SESSION_TTL` config flag.
  This makes it possible to configure the time after which a node-session
  expires after no activity ([#100](https://github.com/brocaar/loraserver/issues/100)).
* Relax frame-counter mode has been changed to disable frame-counter check mode
  to deal with different devices ([#133](https://github.com/brocaar/loraserver/issues/133)).

## 0.17.0

**Features:**

* Add `--extra-frequencies` / `EXTRA_FREQUENCIES` config option to configure
  additional channels (in case supported by the selected ISM band).
* Add `--enable-uplink-channels` / `ENABLE_UPLINK_CHANNELS` config option to
  configure the uplink channels active on the network.
* Make adaptive data-rate (ADR) available to every ISM band.

## 0.16.1

**Bugfixes:**

* Fix getting gateway stats when start timestamp is in an other timezone than
  end timestamp (eg. in case of Europe/Amsterdam when changing from CET to
  CEST).

## 0.16.0

**Note:** LoRa Server now requires a PostgreSQL (9.5+) database to persist the
gateway data. See [getting started](getting-started.md) for more information.

**Features:**

* Gateway management and gateway stats:
    * API methods have been added to manage gateways (including GPS location).
    * GPS location of receiving gateways is added to uplink frames published
      to the application-server.
    * Gateway stats (rx / tx) are aggregated on intervals specified in
      `--gw-stats-aggregation-intervals` (make sure to set the correct
      `--timezone`!).
    * When `--gw-create-on-stats` is set, then gateways will be automatically
      created when receiving gateway stats.
* LoRa Server will retry to connect to the MQTT broker when it isn't available
  (yet) on startup, instead of failing.

## 0.15.1

**Bugfixes:**

* Fix error handling for creating a node-session that already exists
* Fix delete node-session regression introduced in 0.15.0

## 0.15.0

**Features:**

* Node-sessions are now stored by `DevEUI`. Before the node-sessions were stored
  by `DevAddr`. In case a single `DevAddr` is used by multiple nodes, the
  `NwkSKey` is used for retrieving the corresponding node-session.

*Note:* Data will be automatically migrated into the new format. As this process
is not reversible it is recommended to make a backup of the Redis database before
upgrading.

## 0.14.1

**Bugfixes:**

* Add mac-commands (if any) to LoRaWAN frame for Class-C transmissions.

## 0.14.0

**Features:**

* Class C support. When a node is configured as Class-C device, downlink data
  can be pushed to it using the `NetworkServer.PushDataDown` API method.

**Changes:**

* RU 864 - 869 band configuration has been updated (see [#113](https://github.com/brocaar/loraserver/issues/113))

## 0.13.3

**Features:**

* The following band configurations have been added:
    * AS 923
    * CN 779 - 787
    * EU 433
    * KR 920 - 923
    * RU 864 - 869
* Flags for repeater compatibility configuration and dwell-time limitation
  (400ms) have been added (see [configuration](configuration.md))

## 0.13.2

**Features:**

* De-duplication delay can be configured with `--deduplication-delay` or
  `DEDUPLICATION_DELAY` environment variable (default 200ms)
* Get downlink data delay (delay between uplink delivery and getting the
  downlink data from the application server) can be configured with
  `--get-downlink-data-delay`  or `GET_DOWNLINK_DATA_DELAY` environment variable

**Bugfixes:**

* Fix duplicated gateway MAC in application-server and network-controller API
  call

## 0.13.1

**Bugfixes:**

* Fix crash when node has ADR enabled, but it is disabled in LoRa Server

## 0.13.0

**Features:**

* Adaptive data-rate support. See [features](features.md) for information about
  ADR. Note:
  
    * [LoRa App Server](https://docs.loraserver.io/lora-app-server/) 0.2.0 or
      higher is required
    * ADR is currently only implemented for the EU 863-870 ISM band
    * This is an experimental feature

**Fixes:**

* Validate RX2 data-rate (this was causing a panic)

## 0.12.5

**Security:**

* This release fixes a `FCnt` related security issue. Instead of keeping the
  uplink `FCnt` value in sync with the `FCnt` of the uplink transmission, it
  is incremented (uplink `FCnt + 1`) after it has been processed by
  LoRa Server.

## 0.12.4

* Fix regression that caused a FCnt roll-over to result in an invalid MIC
  error. This was caused by validating the MIC before expanding the 16 bit
  FCnt to the full 32 bit value. (thanks @andrepferreira)

## 0.12.3

* Relax frame-counter option.

## 0.12.2

* Implement China 470-510 ISM band.
* Improve logic to decide which gateway to use for downlink transmission.

## 0.12.1

* Fix multiple LoRa Server instances processing the same gateway payloads
  (resulting in the gateway count multiplied by the number of LoRa Server
  instances).

## 0.12.0

This release decouples the node "inventory" part from LoRa Server. This
introduces some breaking (API) changes, but in the end this will make it easier
to integrate LoRa Server into your own platform as you're not limited anymore
by it's datastructure.

### API

Between all LoRa Server project components [gRPC](http://gprc.io) is used
for communication. Optionally, this can be secured by (client) certificates.
The RESTful JSON api and api methods to manage channels, applications and nodes
has been removed from LoRa Server. The node-session api methodds are still
part of LoRa Server, but are only exposed by gRPC.

### Application-server

An application-server component and [API](https://github.com/brocaar/loraserver/blob/master/api/as/as.proto)
was introduced to be responsible for the "inventory" part. This component is
called by LoRa Server when a node tries to join the network, when data is
received and to retrieve data for downlink transmissions.

The inventory part has been migrated to a new project called
[LoRa App Server](http://docs.loraserver.io/lora-app-server/). See it's
changelog for instructions how to migrate.

### Configuration

As components have been dropped and introduced, you'll probably need to update
your LoRa Server configuration. 

### Important

Before upgrading, make sure you have a backup of all data in the PostgreSQL
and Redis database!

## 0.11.0

* Implement receive window (RX1 or RX2) and RX2 data-rate option in node and
  node-session API (and web-interface).

## 0.10.1

* Fix overwriting existing node-session (owned by different DevEUI)
  (thanks @iBrick)

## 0.10.0

* Implement (optional) JWT token authentication and authorization for the gRPC
  and RESTful JSON API. See [api documentation](https://docs.loraserver.io/loraserver/api/).
* Implement support for TLS
* Serve the web-interface, RESTful interface and gRPC interface on the same port
  (defined by `--http-bind`). When TLS is disabled, the gRPC interface is
  served from a different port (defined by `--grpc-insecure-bind`).
* Fix: delete node-session (if it exists) on node delete

## 0.9.2

* Fix Swagger base path.

## 0.9.1

* Fix `cli.ActionFunc` deprecation warning.

## 0.9.0

**WARNING:** if you're using the JSON-RPC interface, this will be a breaking
upgrade, as the JSON-RPC API has been replaced by a gRPC API.

In order to keep the possiblity to access the API from web-based applications
(e.g. the web-interface), a RESTful JSON API has been implemented on top
of the gRPC API (using [grpc-gateway](https://github.com/grpc-ecosystem/grpc-gateway)).

Please refer to the LoRa Server documentation for more information:
[https://docs.loraserver.io/loraserver/api/](https://docs.loraserver.io/loraserver/api/).

## 0.8.2

* Validate the join-request DevEUI belongs to the given AppEUI
* Implement `Node.FlushTXPayloadQueue` API method
* Update `GatewayStatsPacket` struct (`CustomData` and `TXPacketsEmitted`, to
  be implemented by the lora-gateway-bridge).


## 0.8.1

* Bugfix: 'fix unknown channel for frequency' error when using custom-channels (`CFList`)
  (thanks @arjansplit)

## 0.8.0

* Implement network-controller backend
* Implement support for sending and receiving MAC commands (no support for proprietary commands yet)
* Refactor test scenarios
* Web-interface: nodes can now be accessed from the applications tab (nodes button)

**Note:** You need to update to LoRa Semtech Bridge 2.0.1+ or 1.1.4+ since
it fixes a mac command related marshaling issue.

## 0.7.0

* Complete join-accept payload with:
    * RXDelay
    * DLSettings (RX2 data-rate and RX1 data-rate offset)
    * CFList (optional channel-list, see LoRaWAN specs to see if this
      option is available for your region)

  All values can be set / created throught the API or web-interface

## 0.6.1

* Band configuration must now be specified with the ``--band`` argument
  (no more separate binaries per ism band)
* RX info notifications (``application/[AppEUI]/node/[DevEUI]/rxinfo``)

## 0.6.0

* Implement various notifications to the application:
    * Node join accept (``application/[AppEUI]/node/[DevEUI]/join``)
    * Errors (e.g. max payload size exceeded) (``application/[AppEUI]/node/[DevEUI]/error``)
    * ACK of confirmed data down (``application/[AppEUI]/node/[DevEUI]/ack``)
* Handle duplicated downlink payloads (when running multiple LoRa Server instances each server
  is receiving the TXPayload from MQTT, just one needs to handle it)
* New ISM bands:
    * US 902-928 band (thanks @gzwsc2007 for testing)
    * AU 915-928 band (thanks @Mehradzie for implementing and testing)
* Fix: use only one receive-window (thanks @gzwsc2007)

## 0.5.1

* Expose RX RSSI (signal strength) to application
* Provide binaries for multiple platforms

## 0.5.0

Note: this release is incompatible with lora-semtech-bridge <= 1.0.1

* Replaced hardcoded tx related settings by lorawan/band defined variables
* Minor changes to TX / RX structs
* Change gateway encoding to json (from gob encoding)
* Source-code re-structure (internal code is now under `internal/...`,
  exported packet related structs are now under `models/...`)

## 0.4.1

* Update mqtt vendor to fix various connection issues
* Fix shutting down server when mqtt server is unresponsive

## 0.4.0

* Implement confirmed data up
* Implement (confirmed) data down
* Implement graceful shutdown
* Re-subscribe on mqtt connection error (thanks @Magicking)
* Fix FCnt input bug in web-interface (number was casted to a string, which was rejected by the API)

## 0.3.1

* Bugfix related to ``FCnt`` increment (thanks @ivajloip)

## 0.3.0

* MQTT topics updated (`node/[DevEUI]/rx` is now `application/[AppEUI]/node/[DevEUI]/rx`)
* Restructured RPC API (per domain)
* Auto generated API docs (in web-interface)

## 0.2.1

* `lorawan` packet was updated (with MType fix)

## 0.2.0

* Web-interface for application and node management
* *LoRa Server* is now a single binary with embedded migrations and static files

## 0.1.0

* Initial release
