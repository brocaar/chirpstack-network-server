# LoRa Server

[![Build Status](https://travis-ci.org/brocaar/loraserver.svg?branch=master)](https://travis-ci.org/brocaar/loraserver)
[![GoDoc](https://godoc.org/github.com/brocaar/loraserver?status.svg)](https://godoc.org/github.com/brocaar/loraserver)

*LoRa Server* is a LoRaWAN network-server. It is responsible for the
communication with the LoRa gateway(s) and applications. Communication
with the applications and gateways is done over MQTT. Configuration of
applications and nodes can be done with the provided web-interface.

![web-interface](doc/webinterface.jpg)

## Todo

Note: This project is under development. Please test and give feedback but know that things might break! 

- [x] unconfirmed data up
- [x] activation by personalization
- [x] over-the-air activation
- [x] web-interface
- [x] auto-generated RPC documentation
- [ ] freeze initial database schema
- [x] confirmed data up
- [x] data down (confirmed and unconfirmed)
- [ ] handling of mac commands
- [ ] cross band (only the EU 863-870MHz ISM Band is supported right now)
- [ ] cross-platform binary build (only linux amd64 is available right now)


## Getting started

* First install the [*Lora Semtech Bridge*](https://github.com/brocaar/lora-semtech-bridge)

* Download and unpack *LoRa Server*. Pre-compiled binaries can be found on the
  [releases](https://github.com/brocaar/loraserver/releases) page.

* Install a MQTT server (used for communication with the gateways and applications).
  [Mosquitto](http://mosquitto.org/) is a good option.

* Install [PostgreSQL](http://www.postgresql.org/) and create a database
  (used to store application and node data).

* Install [Redis](http://redis.io/) (used to store node sessions).

* Start the *LoRa Server* by executing ``./loraserver``. The ``--help`` argument will show you
  all the available config options. With ``--db-automigrate`` the database schema will be
  created / updated automatically.

* Use the web-interface (when running locally, it is available at
  [http://localhost:8000/](http://localhost:8000/)) to create an application and
  node. You should now be able to use OTAA to activate your node. Alternatively, use the
  web-interface to activate your node (Session / ABP button).

* See [loratestapp](https://github.com/brocaar/loratestapp) for an example application
  implementation.

## Getting started (using ``docker-compose``)

An alternative way to get started (either for development or for testing this project)
is to start this project by using [docker-compose](https://docs.docker.com/compose/).

After cloning this repository, you should be able to start the whole project
(including the *Lora Semtech Bridge*) with:

``$ docker-compose -f docker-compose.yml -f docker-compose.devel.yml up``

## API

*LoRa Server* provides a JSON-RPC API over HTTP. All calls are performend by
sending a post request to the ``/rpc`` endpoint. API documentation is available within
the web-interface.

## MQTT

MQTT is used for communication with the gateways and applications. Since all / most gateways
are using the protocol defined by Semtech (over UDP), the *LoRa Semtech Bridge* is needed.

### Topics

#### ``gateway/[MAC]/rx``

Received packets by the gateway ([``RXPacket``](https://godoc.org/github.com/brocaar/loraserver#RXPacket)).
Data is [Gob](https://golang.org/pkg/encoding/gob/) encoded (this might change as it is a Go specific format).

#### ``gateway/[MAC]/tx``

To-be transmitted packets by the gateway ([``TXPacket``](https://godoc.org/github.com/brocaar/loraserver#TXPacket))
Data is [Gob](https://golang.org/pkg/encoding/gob/) encoded (this might change as it is a Go specific format).

#### ``application/[AppEUI]/node/[DevEUI]/rx``

Received node payloads ([``ApplicationRXPayload``](https://godoc.org/github.com/brocaar/loraserver/application/mqttpubsub#ApplicationRXPayload)).
The payload is JSON encoded and holds a ``data`` key containing the decrypted data received from the node in ``base64`` encoding.

#### ``application/[AppEUI]/node/[DevEUI]/tx``

To-be transmitted payloads to the node (to-be implemented). Data is JSON encoded.

## License

This package is licensed under the MIT license. See ``LICENSE``.
