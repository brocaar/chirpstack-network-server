---
title: Application Server
menu:
    main:
        parent: integrate
        weight: 2
description: Information about the Application Server and how to implement a custom one.
---

# Application Server

ChirpStack Network Server forwards received uplink frames and acknowledgements to a so called
Application Server component. You can either use ChirpStack Application Server
(an Application Server provided by the ChirpStack open-source LoRaWAN<sup>&reg;</sup> Network Server stack), or implement
your own Application Server.

ChirpStack Network Server supports sending data for different devices to different
Application Servers. See [Routing Profile]({{<ref "/features/routing-profile.md">}})
for more information.

## ChirpStack Application Server

[ChirpStack Application Server](/application-server/) is an open-source
implementation of a LoRaWAN Application Server.

## Custom Application Server

It is also possible to implement a custom Application Server. The
Application Server API has been defined as a [gRPC](https://grpc.io) service
which allows you to easily generate stubs for various programming languages.
See the [gRPC](https://grpc.io) site for more information about
the gRPC framework and how to generate source-code using `.proto` files.

Please refer to [api/as/as.proto](https://github.com/brocaar/chirpstack-network-server/blob/master/api/as/as.proto)
for the Application Server API specification. 
For the network-server API, please refer to the [API documentation]({{<ref "/integrate/api.md">}}) for the
for more information.
