# LoRa Server documentation

LoRa Server is an open-source LoRaWAN network-server. It is responsible for
handling (and de-duplication) of uplink data received by the gateway(s)
and the scheduling of downlink data transmissions.

## Project components

This project exists out of multiple components

![architecture](https://www.gliffy.com/go/publish/image/11010339/L.png)

* [lora-gateway-bridge](https://docs.loraserver.io/lora-gateway-bridge/) - converts
  the [packet_forwarder protocol](https://github.com/Lora-net/packet_forwarder/blob/master/PROTOCOL.TXT)
  to MQTT and back
* [loraserver](https://docs.loraserver.io/loraserver/) - LoRaWAN network-server
* [lora-app-server](https://docs.loraserver.io/lora-app-server/) - LoRaWAN
  application-server
* lora-controller (todo) - LoRaWAN network-controller

## Downloads

Pre-compiled binaries are available for:

* Linux (including ARM / Raspberry Pi)
* OS X
* Windows

See [https://github.com/brocaar/loraserver/releases](https://github.com/brocaar/loraserver/releases)
for downloads. Source-code can be found at
[https://github.com/brocaar/loraserver](https://github.com/brocaar/loraserver).

## Sponsors

[![acklio](img/sponsors/acklio.png)](http://www.ackl.io/)

## License

LoRa Server is distributed under the MIT license. See also
[LICENSE](https://github.com/brocaar/loraserver/blob/master/LICENSE).
