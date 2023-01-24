# ChirpStack Network Server

![Tests](https://github.com/brocaar/chirpstack-network-server/actions/workflows/main.yml/badge.svg?branch=master)

ChirpStack Network Server is an open-source LoRaWAN network-server, part of
[ChirpStack](https://www.chirpstack.io/). It is responsible for
handling (and de-duplication) of uplink data received by the gateway(s)
and the scheduling of downlink data transmissions.

## !!! ChirpStack v4 note !!!

With the release of ChirpStack v4, the source-code has been migrated to
[https://github.com/chirpstack/chirpstack/](https://github.com/chirpstack/chirpstack/).
Please refer to the [v3 to v4 migration guide](https://www.chirpstack.io/docs/v3-v4-migration.html)
for information on how to migrate your ChirpStack v3 instance.

## Architecture

![architecture](https://www.chirpstack.io/static/img/graphs/architecture.dot.png)

### Component links

* [ChirpStack Gateway Bridge](https://www.chirpstack.io/gateway-bridge/)
* [ChirpStack Network Server](https://www.chirpstack.io/network-server/)
* [ChirpStack Application Server](https://www.chirpstack.io/application-server/)

## Links

* [Downloads](https://www.chirpstack.io/network-server/overview/downloads/)
* [Docker image](https://hub.docker.com/r/chirpstack/chirpstack-network-server/)
* [Documentation](https://www.chirpstack.io/network-server/) and
  [Getting started](https://www.chirpstack.io/network-server/getting-started/)
* [Building from source](https://www.chirpstack.io/network-server/community/source/)
* [Contributing](https://www.chirpstack.io/network-server/community/contribute/)
* Support
  * [Support forum](https://forum.chirpstack.io)
  * [Bug or feature requests](https://github.com/brocaar/chirpstack-network-server/issues)

## License

ChirpStack Network Server is distributed under the MIT license. See also
[LICENSE](https://github.com/brocaar/chirpstack-network-server/blob/master/LICENSE).
