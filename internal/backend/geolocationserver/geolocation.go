package geolocationserver

import "github.com/brocaar/chirpstack-api/go/v3/geo"

var client geo.GeolocationServerServiceClient

// SetClient sets the given geolocation-server client.
func SetClient(c geo.GeolocationServerServiceClient) {
	client = c
}

// Client returns the geolocation-server client.
func Client() geo.GeolocationServerServiceClient {
	return client
}
