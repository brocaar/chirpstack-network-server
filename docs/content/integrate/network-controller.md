---
title: Network-controller
menu:
  main:
    parent: integrate
    weight: 3
---

# Network-controller

LoRa Server makes it possible to integrate an external network-controller
to interact with the LoRa Server API. This makes it possible to let an external
component schedule mac-commands for example.

For this to work, the external network-controller must implement the
`NetworkController` gRPC service as specified in
[`api/nc/nc.proto`](https://github.com/brocaar/loraserver/blob/master/api/nc/nc.proto).
Also LoRa Server must be configured so that it connects to this network-controller
(see [configuration]({{< ref "install/config.md" >}})).


## Code example in Go

The following code illustrates a network-server which will ask the node its
battery status and link margin, each time an uplink frame is received.


```go
package main

import (
	"fmt"
	"log"
	"net"

	"github.com/brocaar/loraserver/api/nc"
	"github.com/brocaar/loraserver/api/ns"
	"github.com/brocaar/lorawan"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
)

// NetworkControllerAPI implements the NetworkController service.
// https://github.com/brocaar/loraserver/blob/master/api/nc/nc.proto
type NetworkControllerAPI struct {
	nsClient ns.NetworkServerClient
}

// HandleRXInfo handles the RX info calls from the network-server, which is
// called on each uplink.
// Each time this method is called, the code below will schedule a mac-command
// for this node asking for the battery status and link margin
// (DevStatusReq mac-command).
func (n *NetworkControllerAPI) HandleRXInfo(ctx context.Context, req *nc.HandleRXInfoRequest) (*nc.HandleRXInfoResponse, error) {
	var devEUI lorawan.EUI64
	copy(devEUI[:], req.DevEUI)

	log.Printf("HandleRXInfo method called (DevEUI: %s)", devEUI)

	// construct the mac-command
	mac := lorawan.MACCommand{
		CID:     lorawan.DevStatusReq,
		Payload: nil, // this mac-command does not have a payload
	}
	macBytes, err := mac.MarshalBinary()
	if err != nil {
		log.Printf("marshal mac-command error: %s")
		return nil, err
	}

	// send the mac-command to the network-server
	_, err = n.nsClient.EnqueueDataDownMACCommand(ctx, &ns.EnqueueDataDownMACCommandRequest{
		DevEUI: req.DevEUI,
		Cid:    uint32(lorawan.DevStatusReq),
		Commands: [][]byte{
			macBytes,
		},
	})
	if err != nil {
		return nil, err
	}

	log.Printf("requested the status of the end-device %s", devEUI)
	return &nc.HandleRXInfoResponse{}, nil
}

// HandleDataUpMACCommand handles the uplink mac-commands, which is called
// by the network-server each time an acknowledgement was received for a
// mac-command scheduled through the API (as is done in the above method).
// The code below will simply print the battery and margin values.
func (n *NetworkControllerAPI) HandleDataUpMACCommand(ctx context.Context, req *nc.HandleDataUpMACCommandRequest) (*nc.HandleDataUpMACCommandResponse, error) {
	var devEUI lorawan.EUI64
	copy(devEUI[:], req.DevEUI)

	log.Printf("HandleDataUpMACCommand method called (DevEUI: %s)", devEUI)

	// make sure the CID (command identifier) is of type DevStatusAns.
	if req.Cid != uint32(lorawan.DevStatusAns) {
		return nil, fmt.Errorf("unexpected CID: %s", lorawan.CID(req.Cid))
	}

	for _, macBytes := range req.Commands {
		var mac lorawan.MACCommand
		if err := mac.UnmarshalBinary(true, macBytes); err != nil { // true since this is an uplink mac-command
			log.Printf("unmarshal mac-command error: %s", err)
			return nil, err
		}

		pl, ok := mac.Payload.(*lorawan.DevStatusAnsPayload)
		if !ok {
			return nil, fmt.Errorf("expected *lorawan.DevStatusAnsPayload, got %T", mac.Payload)
		}

		log.Printf("battery: %d, margin: %d (DevEUI: %s)", pl.Battery, pl.Margin, devEUI)
	}

	return &nc.HandleDataUpMACCommandResponse{}, nil
}

func main() {
	// allow insecure / non-tls connections
	grpcDialOpts := []grpc.DialOption{
		grpc.WithInsecure(),
	}

	// connect to loraserver on localhost:8000
	nsConn, err := grpc.Dial("localhost:8000", grpcDialOpts...)
	if err != nil {
		log.Fatal(err)
	}

	// create the NetworkServer client
	nsClient := ns.NewNetworkServerClient(nsConn)

	// create the NetWorkController service
	ncService := NetworkControllerAPI{
		nsClient: nsClient,
	}

	// setup the gRPC server
	grpcServerOpts := []grpc.ServerOption{}
	ncServer := grpc.NewServer(grpcServerOpts...)
	nc.RegisterNetworkControllerServer(ncServer, &ncService)
	ln, err := net.Listen("tcp", "0.0.0.0:8002")
	if err != nil {
		log.Fatal(err)
	}

	// start the gRPC api server
	log.Fatal(ncServer.Serve(ln))
}
```
