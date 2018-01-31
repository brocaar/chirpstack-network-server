---
title: Application server
menu:
    main:
        parent: integrate
        weight: 2
---

## Application-server

LoRa Server forwards received uplink frames and acknowledgements to a so called
application-server component. You can either use LoRa App Server
(an application-server provided by the LoRa Server project), or implement
your own application-server.

LoRa Server supports sending data for different devices to different
application-servers. See [routing-profile]({{<ref "features/routing-profile.md">}})
for more information.

### LoRa App Server

[LoRa App Server](https://docs.loraserver.io/lora-app-server/) is a reference
implementation of an application-server compatible with LoRa Server.

### Your own application-server

It is also possible to implement your own application-server. The
application-server API has been defined as a [gRPC](https://grpc.io) service
so that it should be really easy to generate stubs for various programming
languages. See the [gRPC](https://grpc.io) site for more information about
the gRPC framework and how to transform `.proto` files into source-code.
Please see [api/as/as.proto](https://github.com/Frankz/loraserver/blob/master/api/as/as.proto)
for the application-server API specification.

See also the [api documentation]({{<ref "integrate/api.md">}}) for the
network-server API that your application-server can use.
