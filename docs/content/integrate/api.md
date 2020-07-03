---
title: API
menu:
    main:
        parent: integrate
        weight: 1
description: Instructions how to use the API provided by ChirpStack Network Server and integrate this with your services.
---

# API

The ChirpStack Network Server components are using [gRPC](http://www.grpc.io) for
inter-component communication. The definitions of these interfaces can be
found in the form of `.proto` files in the [API](https://github.com/brocaar/chirpstack-api/tree/master/protobuf)
folder of the source repository:

* [api/as/as.proto](https://github.com/brocaar/chirpstack-api/blob/master/protobuf/as/as.proto): Application Server interface
* [api/geo/geo.proto](https://github.com/brocaar/chirpstack-api/blob/master/protobuf/geo/geo.proto): Geolocation Server interface
* [api/ns/ns.proto](https://github.com/brocaar/chirpstack-api/blob/master/protobuf/ns/ns.proto): Network Server interface
* [api/nc/nc.proto](https://github.com/brocaar/chirpstack-api/blob/master/protobuf/nc/nc.proto): Network Controller interface

## Client / server stubs

Each subdirectory (e.g. `ns`, `as` or `nc`) provides Go client code and
server stubs, which means you can import these as packages when using Go.
When using other programming languages, you'll need to generate the client
and / or server stubs yourself (which is thanks to gRPC fairly easy).

gRPC has currently support for: C++, Java, Python, Go, Ruby, C#, Node.js,
Android Java, Objective-C and PHP.

Please refer to the [gRPC getting started](http://www.grpc.io/docs/quickstart/)
guide for more information.
