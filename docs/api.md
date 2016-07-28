# API

LoRa Server provides a [gRPC](http://www.grpc.io/) API for easy integration
with your own platform. On top of this gRPC API, LoRa Server provides a
RESTful JSON interface, so that you can use the API for web-applications
(note that gRPC is a binary API layer, on top of
[protocol-buffers](https://developers.google.com/protocol-buffers/).

!!! info "Protocol-buffer files"
	LoRa Server provides a gRPC client for Go. For other programming languages
	you can use the .proto files inside the [api](https://github.com/brocaar/loraserver/tree/master/api)
	folder for generating clients. See the [gRPC](http://www.grpc.io/) documentation
	for documentation.


## gRPC interface

The following example creates an application and node using the gRPC API:

```go
package main

import (
	"log"

	"golang.org/x/net/context"
	"google.golang.org/grpc"

	"github.com/brocaar/loraserver/api"
)

func main() {
	conn, err := grpc.Dial("localhost:9000", grpc.WithInsecure())
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	appClient := api.NewApplicationClient(conn)
	nodeClient := api.NewNodeClient(conn)

	_, err = appClient.Create(context.Background(), &api.CreateApplicationRequest{
		AppEUI: "0102030405060708",
		Name:   "example app",
	})
	if err != nil {
		log.Fatal(err)
	}

	_, err = nodeClient.Create(context.Background(), &api.CreateNodeRequest{
		DevEUI: "0807060504030201",
		AppEUI: "0102030405060708",
		AppKey: "01020304050607080102030405060708",
	})
	if err != nil {
		log.Fatal(err)
	}
}
```

The following clients are available:

* [ApplicationClient](https://godoc.org/github.com/brocaar/loraserver/api#ApplicationClient)
* [NodeClient](https://godoc.org/github.com/brocaar/loraserver/api#NodeClient)
* [NodeSessionClient](https://godoc.org/github.com/brocaar/loraserver/api#NodeSessionClient)
* [ChannelClient](https://godoc.org/github.com/brocaar/loraserver/api#ChannelClient)
* [ChannelListClient](https://godoc.org/github.com/brocaar/loraserver/api#ChannelListClient)


## RESTful JSON interface

Since gRPC [can't be used in browsers](http://www.grpc.io/faq/), LoRa Server
provides a RESTful JSON interface (by using [grpc-gateway](https://github.com/grpc-ecosystem/grpc-gateway))
on top of the gRPC interface, exposing the same API methods as the gRPC API.
The JSON API documentation can be found at `/api/v1`.

![Swagger API](img/swaggerapi.jpg)
