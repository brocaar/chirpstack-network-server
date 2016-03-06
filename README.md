# LoRaWAN network-server

[![Build Status](https://travis-ci.org/brocaar/loraserver.svg?branch=master)](https://travis-ci.org/brocaar/loraserver)
[![GoDoc](https://godoc.org/github.com/brocaar/loraserver?status.svg)](https://godoc.org/github.com/brocaar/loraserver)

*loraserver* is a LoRaWAN network-service. It is responsible for the
communication with the LoRa gateway(s) and applications. Communication
with the applications and gateways is done over MQTT.


## Todo

Note: This project is under development. Please test and give feedback but know that things might break! 

- [x] unconfirmed data up
- [x] activation by personalization
- [x] over-the-air activation
- [ ] confirmed data up
- [ ] data down (confirmed and unconfirmed)
- [ ] handling of mac commands
- [ ] cross band (only the EU 863-870MHz ISM Band is supported right now)
- [ ] cross-platform binary build (only linux amd64 is available right now)


## Getting started

* First install the *Lora Semtech Bridge* (https://github.com/brocaar/lora-semtech-bridge)

* Download and unpack ``loraserver``: https://github.com/brocaar/loraserver/releases.

* Install a MQTT server (used for communication with the gateways and applications).
  Mosquitto is a good option: http://mosquitto.org/.

* Install PostgreSQL (used to store application and node data).

* Install Redis (used to store node sessions).

* Start the ``loraserver`` service. The ``--help`` argument will show you all the available
  config options. 

* See https://github.com/brocaar/loratestapp for an example application implementation.

## API

The *loraserver* provides a JSON-RPC API over HTTP. All calls are performend by
sending a post request to the ``/rpc`` endpoint.

### Application

#### Create
```json
{"method": "API.CreateApplication", "params":[{"appEUI": "0102030405060708", "name": "test application"}]}
```

#### Get
```json
{"method": "API.GetApplication", "params":["0102030405060708"]}
```

#### Update
```json
{"method": "API.UpdateApplication", "params":[{"appEUI": "0102030405060708", "name": "test application 2"}]}
```

#### Delete
```json
{"method": "API.DeleteApplication", "params":["0102030405060708"]}
```

### Node

#### Create
```json
{"method": "API.CreateNode", "params":[{"devEUI": "0807060504030201", "appEUI": "0102030405060708", "appKey": "01020304050607080910111213141516"}]}
```

#### Get
```json
{"method": "API.GetNode", "params":["0807060504030201"]}
```

#### Update
```json
{"method": "API.UpdateNode", "params":[{"devEUI": "0807060504030201", "appEUI": "0102030405060708", "appKey": "01010101010101010101010101010101"}]}
```

#### Delete
```json
{"method": "API.DeleteNode", "params":["0807060504030201"]}
```

### Node sessions (active nodes)

#### Get
```json
{"method": "API.GetNodeSession", "params":["01020304"]}
```

#### Create (activation by personalization)
```json
{"method": "API.CreateNodeSession", "params":[{"devAddr": "01020304", "devEUI": "0807060504030201", "appEUI": "0102030405060708", "appSKey": "01010101010101010101010101010101", "nwkSKey": "02020202020202020202020202020202", "fCntUp": 0, "fCntDown": 0}]}
```

#### Delete
```json
{"method": "API.DeleteNodeSession", "params":["01020304"]}
```

## License

This package is licensed under the MIT license. See ``LICENSE``.
