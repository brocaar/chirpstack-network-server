# LoRa Server documentation

LoRa Server is a LoRaWAN network-server. It is responsible for the
communication with the LoRa gateway(s) and applications.
Communication with the applications and gateways is done over MQTT.
Configuration of applications and nodes can be done with the provided
web-interface or the JSON-RPC API.

![Webinterface](img/webinterface.jpg)

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
- JSON-RPC API (see web-interface for documentation)
- ISM bands
	- EU 863-870
	- US 902-928
	- AU 915-928

## Downloads

Pre-compiled binaries are available for:

* Linux (including ARM / Raspberry Pi)
* OS X
* Windows

See [https://github.com/brocaar/loraserver/releases](https://github.com/brocaar/loraserver/releases)
for downloads. Source-code can be found at
[https://github.com/brocaar/loraserver](https://github.com/brocaar/loraserver).

## License

LoRa Server is distributed under the MIT license. See also
[LICENSE](https://github.com/brocaar/loraserver/blob/master/LICENSE).
