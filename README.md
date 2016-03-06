# LoRaWAN network-server

[![Build Status](https://travis-ci.org/brocaar/loraserver.svg?branch=master)](https://travis-ci.org/brocaar/loraserver)
[![GoDoc](https://godoc.org/github.com/brocaar/loraserver?status.svg)](https://godoc.org/github.com/brocaar/loraserver)

*loraserver* is a LoRaWAN network-service. It is responsible for the
communication with the LoRa gateway(s) and applications. Communication
with the applications and gateways is done over MQTT.

## Getting started

* First install the *Lora Semtech Bridge* (https://github.com/brocaar/lora-semtech-bridge)

* Install ``loraserver``:

```bash
$ go get github.com/brocaar/loraserver/...
```

* Make sure you have a MQTT server running. Mosquitto is a good option: http://mosquitto.org/.

* Make sure you have a PostgreSQL database running. The PostgreSQL database is used to
  store the application, node and *activation by personalization* (ABP) data.

* Make sure you have a Redis server running. Redis is used to store the node sessions.
  When inserting items in the ``node_abp`` table (PostgreSQL), then ``loraserver`` will
  automatically create node sessions when starting with ``--create-abp-node-sessions``.

* Start the ``loraserver`` service. The ``--help`` argument will show you all the available
  config options. When installing with ``go get``, you will find the migration files under
  ``$GOPATH/src/github.com/brocaar/loraserver/migrations``.

* See https://github.com/brocaar/loratestapp for an example application implementation.

## Todo

- [ ] better documentation :-)
- [x] unconfirmed data up
- [ ] unconfirmed and confirmed data down
- [ ] confirmed data up
- [x] OTAA
- [ ] MAC commands
- [ ] API for ABP
- [ ] GUI for application, node and ABP management?

## License

This package is licensed under the MIT license which can be found in ``LICENSE``.
