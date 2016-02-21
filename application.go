package loraserver

import "github.com/brocaar/lorawan"

// Application contains the information of an application.
type Application struct {
	AppEUI lorawan.EUI64
	Name   string
}
