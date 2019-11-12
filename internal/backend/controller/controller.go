package controller

import (
	"github.com/brocaar/chirpstack-api/go/nc"
)

var client nc.NetworkControllerServiceClient

// init sets the NopNetworkControllerClient by default, as the client is optional.
func init() {
	client = &NopNetworkControllerClient{}
}

// SetClient sets up the given controller client.
func SetClient(c nc.NetworkControllerServiceClient) {
	client = c
}

// Client returns the controller cient.
func Client() nc.NetworkControllerServiceClient {
	return client
}
