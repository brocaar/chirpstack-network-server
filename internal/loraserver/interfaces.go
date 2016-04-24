package loraserver

import (
	"github.com/brocaar/loraserver/models"
	"github.com/brocaar/lorawan"
)

// GatewayBackend is the interface of a gateway backend.
// A gateway backend is responsible for the communication with the gateway.
type GatewayBackend interface {
	Send(models.TXPacket) error         // send the given packet to the gateway
	RXPacketChan() chan models.RXPacket // channel containing the received packets
	Close() error                       // close the gateway backend.
}

// ApplicationBackend is the interface of an application backend.
// An application backend is responsible forwarding data to the application
// and receiving data that should be sent to the node.
type ApplicationBackend interface {
	Send(devEUI, appEUI lorawan.EUI64, payload models.RXPayload) error                           // send the given payload to the application
	Notify(devEUI, appEUI lorawan.EUI64, typ models.NotificationType, payload interface{}) error // send the given notification to the application
	TXPayloadChan() chan models.TXPayload                                                        // channel containing the received payloads from the application
	Close() error                                                                                // close the application backend
}

// NodeManager is the interface for managing the known nodes to the system.
type NodeManager interface {
	create(n models.Node) error                       // create creates a given Node
	update(n models.Node) error                       // update updates the given Node.
	delete(devEUI lorawan.EUI64) error                // delete deletes the Node matching the given DevEUI.
	get(devEUI lorawan.EUI64) (models.Node, error)    // get returns the Node for the given DevEUI.
	getList(limit, offset int) ([]models.Node, error) // getList returns a slice of nodes, sorted by DevEUI.
}

// NodeApplicationsManager is the interface for managing the applications to which nodes can belong.
type NodeApplicationsManager interface {
	create(n models.Application) error                       // create creates a given Application.
	update(n models.Application) error                       // update updates the given Application.
	delete(appEUI lorawan.EUI64) error                       // delete deletes the Application matching the given EUI.
	get(appEUI lorawan.EUI64) (models.Application, error)    // get returns the Application for the given EUI.
	getList(limit, offset int) ([]models.Application, error) // getList returns a slice of applications.
}
