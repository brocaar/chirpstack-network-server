---
title: Changelog
menu:
    main:
        parent: overview
        weight: 3
---

## Changelog

### 0.19.0

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

### 0.18.0

**Features:**

* Add configuration option to log all uplink / downlink frames into a database
  (`--log-node-frames` / `LOG_NODE_FRAMES`).

### 0.17.2

**Bugfixes:**

* Do not reset downlink frame-counter in case of relax frame-counter mode as
  this would also reset the downlink counter on a re-transmit.

### 0.17.1

**Features:**

* TTL of node-sessions in Redis is now configurable through
  `--node-session-ttl` / `NODE_SESSION_TTL` config flag.
  This makes it possible to configure the time after which a node-session
  expires after no activity ([#100](https://github.com/brocaar/loraserver/issues/100)).
* Relax frame-counter mode has been changed to disable frame-counter check mode
  to deal with different devices ([#133](https://github.com/brocaar/loraserver/issues/133)).

### 0.17.0

**Features:**

* Add `--extra-frequencies` / `EXTRA_FREQUENCIES` config option to configure
  additional channels (in case supported by the selected ISM band).
* Add `--enable-uplink-channels` / `ENABLE_UPLINK_CHANNELS` config option to
  configure the uplink channels active on the network.
* Make adaptive data-rate (ADR) available to every ISM band.

### 0.16.1

**Bugfixes:**

* Fix getting gateway stats when start timestamp is in an other timezone than
  end timestamp (eg. in case of Europe/Amsterdam when changing from CET to
  CEST).

### 0.16.0

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

### 0.15.1

**Bugfixes:**

* Fix error handling for creating a node-session that already exists
* Fix delete node-session regression introduced in 0.15.0

### 0.15.0

**Features:**

* Node-sessions are now stored by `DevEUI`. Before the node-sessions were stored
  by `DevAddr`. In case a single `DevAddr` is used by multiple nodes, the
  `NwkSKey` is used for retrieving the corresponding node-session.

*Note:* Data will be automatically migrated into the new format. As this process
is not reversible it is recommended to make a backup of the Redis database before
upgrading.

### 0.14.1

**Bugfixes:**

* Add mac-commands (if any) to LoRaWAN frame for Class-C transmissions.

### 0.14.0

**Features:**

* Class C support. When a node is configured as Class-C device, downlink data
  can be pushed to it using the `NetworkServer.PushDataDown` API method.

**Changes:**

* RU 864 - 869 band configuration has been updated (see [#113](https://github.com/brocaar/loraserver/issues/113))

### 0.13.3

**Features:**

* The following band configurations have been added:
    * AS 923
    * CN 779 - 787
    * EU 433
    * KR 920 - 923
    * RU 864 - 869
* Flags for repeater compatibility configuration and dwell-time limitation
  (400ms) have been added (see [configuration](configuration.md))

### 0.13.2

**Features:**

* De-duplication delay can be configured with `--deduplication-delay` or
  `DEDUPLICATION_DELAY` environment variable (default 200ms)
* Get downlink data delay (delay between uplink delivery and getting the
  downlink data from the application server) can be configured with
  `--get-downlink-data-delay`  or `GET_DOWNLINK_DATA_DELAY` environment variable

**Bugfixes:**

* Fix duplicated gateway MAC in application-server and network-controller API
  call

### 0.13.1

**Bugfixes:**

* Fix crash when node has ADR enabled, but it is disabled in LoRa Server

### 0.13.0

**Features:**

* Adaptive data-rate support. See [features](features.md) for information about
  ADR. Note:
  
    * [LoRa App Server](https://docs.loraserver.io/lora-app-server/) 0.2.0 or
      higher is required
    * ADR is currently only implemented for the EU 863-870 ISM band
    * This is an experimental feature

**Fixes:**

* Validate RX2 data-rate (this was causing a panic)

### 0.12.5

**Security:**

* This release fixes a `FCnt` related security issue. Instead of keeping the
  uplink `FCnt` value in sync with the `FCnt` of the uplink transmission, it
  is incremented (uplink `FCnt + 1`) after it has been processed by
  LoRa Server.

### 0.12.4

* Fix regression that caused a FCnt roll-over to result in an invalid MIC
  error. This was caused by validating the MIC before expanding the 16 bit
  FCnt to the full 32 bit value. (thanks @andrepferreira)

### 0.12.3

* Relax frame-counter option.

### 0.12.2

* Implement China 470-510 ISM band.
* Improve logic to decide which gateway to use for downlink transmission.

### 0.12.1

* Fix multiple LoRa Server instances processing the same gateway payloads
  (resulting in the gateway count multiplied by the number of LoRa Server
  instances).

### 0.12.0

This release decouples the node "inventory" part from LoRa Server. This
introduces some breaking (API) changes, but in the end this will make it easier
to integrate LoRa Server into your own platform as you're not limited anymore
by it's datastructure.

#### API

Between all LoRa Server project components [gRPC](http://gprc.io) is used
for communication. Optionally, this can be secured by (client) certificates.
The RESTful JSON api and api methods to manage channels, applications and nodes
has been removed from LoRa Server. The node-session api methodds are still
part of LoRa Server, but are only exposed by gRPC.

#### Application-server

An application-server component and [API](https://github.com/brocaar/loraserver/blob/master/api/as/as.proto)
was introduced to be responsible for the "inventory" part. This component is
called by LoRa Server when a node tries to join the network, when data is
received and to retrieve data for downlink transmissions.

The inventory part has been migrated to a new project called
[LoRa App Server](http://docs.loraserver.io/lora-app-server/). See it's
changelog for instructions how to migrate.

#### Configuration

As components have been dropped and introduced, you'll probably need to update
your LoRa Server configuration. 

#### Important

Before upgrading, make sure you have a backup of all data in the PostgreSQL
and Redis database!

### 0.11.0

* Implement receive window (RX1 or RX2) and RX2 data-rate option in node and
  node-session API (and web-interface).

### 0.10.1

* Fix overwriting existing node-session (owned by different DevEUI)
  (thanks @iBrick)

### 0.10.0

* Implement (optional) JWT token authentication and authorization for the gRPC
  and RESTful JSON API. See [api documentation](https://docs.loraserver.io/loraserver/api/).
* Implement support for TLS
* Serve the web-interface, RESTful interface and gRPC interface on the same port
  (defined by `--http-bind`). When TLS is disabled, the gRPC interface is
  served from a different port (defined by `--grpc-insecure-bind`).
* Fix: delete node-session (if it exists) on node delete

### 0.9.2

* Fix Swagger base path.

### 0.9.1

* Fix `cli.ActionFunc` deprecation warning.

### 0.9.0

**WARNING:** if you're using the JSON-RPC interface, this will be a breaking
upgrade, as the JSON-RPC API has been replaced by a gRPC API.

In order to keep the possiblity to access the API from web-based applications
(e.g. the web-interface), a RESTful JSON API has been implemented on top
of the gRPC API (using [grpc-gateway](https://github.com/grpc-ecosystem/grpc-gateway)).

Please refer to the LoRa Server documentation for more information:
[https://docs.loraserver.io/loraserver/api/](https://docs.loraserver.io/loraserver/api/).

### 0.8.2

* Validate the join-request DevEUI belongs to the given AppEUI
* Implement `Node.FlushTXPayloadQueue` API method
* Update `GatewayStatsPacket` struct (`CustomData` and `TXPacketsEmitted`, to
  be implemented by the lora-gateway-bridge).


### 0.8.1

* Bugfix: 'fix unknown channel for frequency' error when using custom-channels (`CFList`)
  (thanks @arjansplit)

### 0.8.0

* Implement network-controller backend
* Implement support for sending and receiving MAC commands (no support for proprietary commands yet)
* Refactor test scenarios
* Web-interface: nodes can now be accessed from the applications tab (nodes button)

**Note:** You need to update to LoRa Semtech Bridge 2.0.1+ or 1.1.4+ since
it fixes a mac command related marshaling issue.

### 0.7.0

* Complete join-accept payload with:
    * RXDelay
    * DLSettings (RX2 data-rate and RX1 data-rate offset)
    * CFList (optional channel-list, see LoRaWAN specs to see if this
      option is available for your region)

  All values can be set / created throught the API or web-interface

### 0.6.1

* Band configuration must now be specified with the ``--band`` argument
  (no more separate binaries per ism band)
* RX info notifications (``application/[AppEUI]/node/[DevEUI]/rxinfo``)

### 0.6.0

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

### 0.5.1

* Expose RX RSSI (signal strength) to application
* Provide binaries for multiple platforms

### 0.5.0

Note: this release is incompatible with lora-semtech-bridge <= 1.0.1

* Replaced hardcoded tx related settings by lorawan/band defined variables
* Minor changes to TX / RX structs
* Change gateway encoding to json (from gob encoding)
* Source-code re-structure (internal code is now under `internal/...`,
  exported packet related structs are now under `models/...`)

### 0.4.1

* Update mqtt vendor to fix various connection issues
* Fix shutting down server when mqtt server is unresponsive

### 0.4.0

* Implement confirmed data up
* Implement (confirmed) data down
* Implement graceful shutdown
* Re-subscribe on mqtt connection error (thanks @Magicking)
* Fix FCnt input bug in web-interface (number was casted to a string, which was rejected by the API)

### 0.3.1

* Bugfix related to ``FCnt`` increment (thanks @ivajloip)

### 0.3.0

* MQTT topics updated (`node/[DevEUI]/rx` is now `application/[AppEUI]/node/[DevEUI]/rx`)
* Restructured RPC API (per domain)
* Auto generated API docs (in web-interface)

### 0.2.1

* `lorawan` packet was updated (with MType fix)

### 0.2.0

* Web-interface for application and node management
* *LoRa Server* is now a single binary with embedded migrations and static files

### 0.1.0

* Initial release
