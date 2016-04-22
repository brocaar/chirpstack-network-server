# LoRa Server

[![Build Status](https://travis-ci.org/brocaar/loraserver.svg?branch=master)](https://travis-ci.org/brocaar/loraserver)
[![GoDoc](https://godoc.org/github.com/brocaar/loraserver?status.svg)](https://godoc.org/github.com/brocaar/loraserver)

*LoRa Server* is a LoRaWAN network-server. It is responsible for the
communication with the LoRa gateway(s) and applications. Communication
with the applications and gateways is done over MQTT. Configuration of
applications and nodes can be done with the provided web-interface or JSON-RPC API.

![web-interface](doc/webinterface.jpg)

## Features

Note: This project is under development. Please test and give feedback but know that things might break! 

- [x] unconfirmed data up
- [x] confirmed data up
- [x] data down (confirmed and unconfirmed)
- [x] activation by personalization
- [x] over-the-air activation
- [x] web-interface
- [x] JSON-RPC API (documentation can be found in the web-interface)
- [ ] handling of mac commands
- [ ] cross band (only the EU 863-870MHz ISM Band is supported right now)
- [x] cross-platform binary build (arm / Raspberry Pi, OS X, Linux and Windows are provided)

## Documentation

See the [wiki](https://github.com/brocaar/loraserver/wiki/Getting-started)
for documentation about setting up the LoRa Server and receiving and sending
data.

## License

This package is licensed under the MIT license. See ``LICENSE``.
