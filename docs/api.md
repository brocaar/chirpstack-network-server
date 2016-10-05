# API

The LoRa Server components are using [gRPC](http://www.grpc.io) for 
inter-component communication. The definitions of these interfaces can be
found in in the form of `.proto` files in the the [api](https://github.com/brocaar/loraserver/tree/master/api)
folder of the source repository:

* `api/ns/ns.proto`: network-server interface
* `api/as/as.proto`: application-server interface
* `api/nc/nc.proto`: network-controller interface

# Client / server stubs

Each subdirectory (e.g. `ns`, `as` or `nc`) provides Go client code and
server stubs, which means you can import these as packages when using Go.
When using other programming languages, you'll need to generate the client
and / or server stubs yourself (which is thanks to gRPC fairly easy). 

gRPC has currently support for: C++, Java, Python, Go, Ruby, C#, Node.js,
Android Java, Objective-C and PHP.

Please refer to the [gRPC getting started](http://www.grpc.io/docs/quickstart/)
guide for more information.
