# LoRa Server

[![Build Status](https://travis-ci.org/brocaar/loraserver.svg?branch=master)](https://travis-ci.org/brocaar/loraserver)
[![GoDoc](https://godoc.org/github.com/brocaar/loraserver?status.svg)](https://godoc.org/github.com/brocaar/loraserver)

*LoRa Server* is a LoRaWAN network-server. It is responsible for the
communication with the LoRa gateway(s) and applications. Communication
with the applications and gateways is done over MQTT. Configuration of
applications and nodes can be done with the provided web-interface or JSON-RPC API.

![web-interface](docs/img/webinterface.jpg)


## Features

Note: This project is under development.
Please test and give feedback but know that things might break!

Currently implemented:

- (unconfirmed) data up
- (confirmed) data down
- activation by personalization (ABP)
- over-the-air activation (OTAA)
- sending / receiving of MAC commands
- web-interface
- gRPC and RESTful JSON api (see [API](https://docs.loraserver.io/loraserver/api/))
- ISM bands
	- EU 863-870
	- US 902-928
	- AU 915-928
	- Testers needed: EU 433 ([issues/49](https://github.com/brocaar/loraserver/issues/49))
	- Testers needed: CN 470-510 ([issues/42](https://github.com/brocaar/loraserver/issues/42))

## Documentation

See [http://docs.loraserver.io/loraserver/](http://docs.loraserver.io/loraserver/)
for documentation about setting up the LoRa Server and receiving and sending
data.

## Downloads

Pre-compiled binaries are available for:

* Linux (and ARM build for e.g. Raspberry Pi)
* OS X
* Windows

See [releases](https://github.com/brocaar/loraserver/releases).

## License

LoRa Server is licensed under the MIT license. See ``LICENSE``.
