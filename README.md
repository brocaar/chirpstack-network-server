# LoRa Server

[![Build Status](https://travis-ci.org/brocaar/loraserver.svg?branch=master)](https://travis-ci.org/brocaar/loraserver)
[![GoDoc](https://godoc.org/github.com/brocaar/loraserver?status.svg)](https://godoc.org/github.com/brocaar/loraserver)
[![Gitter chat](https://badges.gitter.im/Join%20Chat.svg)](https://gitter.im/loraserver/loraserver)

LoRa Server is an open-source LoRaWAN network-server. It is responsible for
handling (and de-duplication) of uplink data received by the gateway(s)
and the scheduling of downlink data transmissions.

## Project components

![architecture](https://docs.loraserver.io/img/architecture.png)

* [lora-gateway-bridge](https://docs.loraserver.io/lora-gateway-bridge) - converts
  the [packet_forwarder protocol](https://github.com/Lora-net/packet_forwarder/blob/master/PROTOCOL.TXT)
  to JSON over MQTT and back
* [loraserver](https://docs.loraserver.io/loraserver/) - LoRaWAN network-server
* [lora-app-server](https://docs.loraserver.io/lora-app-server/) - LoRaWAN
  application-server

## Documentation

Please refer to the [documentation](https://docs.loraserver.io/loraserver/) for the
[getting started](https://docs.loraserver.io/loraserver/getting-started/)
documentation and implemented [features](https://docs.loraserver.io/loraserver/features/).

## Downloads

* Pre-compiled binaries are available at the [releases](https://github.com/brocaar/loraserver/releases) page:

	* Linux (including ARM / Raspberry Pi)
	* OS X
	* Windows

* Debian and Ubuntu packages are available at [https://repos.loraserver.io](https://repos.loraserver.io/).
* Source-code can be found at [https://github.com/brocaar/loraserver](https://github.com/brocaar/loraserver).

## Building from source

The easiest way to get started is by using the provided 
[docker-compose](https://docs.docker.com/compose/) environment. To start a bash
shell within the docker-compose environment, execute the following command from
the root of this project:

```bash
docker-compose run --rm loraserver bash
```

A few example commands that you can run:

```bash
# run the tests
make test

# compile
make build

# cross-compile for Linux ARM
GOOS=linux GOARCH=arm make build

# cross-compile for Windows AMD64
GOOS=windows BINEXT=.exe GOARCH=amd64 make build

# build the .tar.gz file
make package

# build the .tar.gz file for Linux ARM
GOOS=linux GOARCH=arm make package

# build the .tar.gz file for Windows AMD64
GOOS=windows BINEXT=.exe GOARCH=amd64 make package
```

Alternatively, you can run the same commands from any working
[Go](https://golang.org/) environment. As all requirements are vendored,
there is no need to `go get` these. Make sure you have Go 1.7.x installed
and that you clone this repository to
`$GOPATH/src/github.com/brocaar/loraserver`.

## Contributing

There are a couple of ways to get involved:

* Join the discussions and share your feedback at [https://gitter.io/loraserver/loraserver](https://gitter.io/loraserver/loraserver)
* Report bugs or make feature-requests by opening an issue at [https://github.com/brocaar/loraserver/issues](https://github.com/brocaar/loraserver/issues)
* Fix issues or improve documentation by creating pull-requests

When you would like to add new features, please discuss the feature first
by creating an issue describing your feature, how you're planning to implement
it, what the usecase is etc...

## Sponsors

[![CableLabs](https://www.loraserver.io/img/sponsors/cablelabs.png)](https://www.cablelabs.com/)
[![acklio](https://www.loraserver.io/img/sponsors/cablelabs.png)](http://www.ackl.io/)

Would you like to support this project too? Please [get in touch](mailto:info@brocaar.com)!

## License

LoRa Server is distributed under the MIT license. See also
[LICENSE](https://github.com/brocaar/loraserver/blob/master/LICENSE).
