# Application server

The application-server is responsible for the node "inventory" part of a
LoRaWAN infrastructure, handling of received application payloads and
the downlink application payload queue. This component has been implemented by
[LoRa App Server](http://docs.loraserver.io/lora-app-server/).

In case you would like to implement a custom application server, please refer
to the [api/as/as.proto](https://github.com/brocaar/loraserver/tree/master/api/as/as.proto)
file. See also the [api](api.md) documentation for more information.
