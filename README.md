# LoRa Server

[![Build Status](https://travis-ci.org/brocaar/loraserver.svg?branch=master)](https://travis-ci.org/brocaar/loraserver)
[![GoDoc](https://godoc.org/github.com/brocaar/loraserver?status.svg)](https://godoc.org/github.com/brocaar/loraserver)
[![Gitter chat](https://badges.gitter.im/Join%20Chat.svg)](https://gitter.im/loraserver/loraserver)

LoRa Server is an open-source LoRaWAN network-server. It is responsible for
handling (and de-duplication) of uplink data received by the gateway(s)
and the scheduling of downlink data transmissions.

## Project components

This project exists out of multiple components

![architecture](https://www.gliffy.com/go/publish/image/11010339/L.png)

* [lora-gateway-bridge](https://docs.loraserver.io/brocaar/lora-gateway-bridge) - converts
  the [packet_forwarder protocol](https://github.com/Lora-net/packet_forwarder/blob/master/PROTOCOL.TXT)
  to MQTT and back
* [loraserver](https://docs.loraserver.io/loraserver/) - LoRaWAN network-server
* [lora-app-server](https://docs.loraserver.io/lora-app-server/) - LoRaWAN
  application-server
* lora-controller (todo) - LoRaWAN network-controller

## Documentation

Please refer to the [documentation](https://docs.loraserver.io/loraserver/) for the
[getting started](https://docs.loraserver.io/loraserver/getting-started/)
documentation and implemented [features](https://docs.loraserver.io/loraserver/features/).

## Downloads

Pre-compiled binaries are available for:

* Linux (including ARM / Raspberry Pi)
* OS X
* Windows

See [https://github.com/brocaar/loraserver/releases](https://github.com/brocaar/loraserver/releases)
for downloads. Source-code can be found at
[https://github.com/brocaar/loraserver](https://github.com/brocaar/loraserver).

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
GOOS=linux GOARCH=arm make build

# build the .tar.gz file for Windows AMD64
GOOS=windows BINEXT=.exe GOARCH=amd64 make build
```

Alternatively, you can run the same commands from any working
[Go](https://golang.org/) environment. As all requirements are vendored,
there is no need to `go get` these, but make sure vendoring is enabled for
your Go environment or that you have Go 1.6+ installed.

## Sponsors

[![acklio](docs/img/sponsors/acklio.png)](http://www.ackl.io/)

## License

LoRa Server is distributed under the MIT license. See also
[LICENSE](https://github.com/brocaar/loraserver/blob/master/LICENSE).
