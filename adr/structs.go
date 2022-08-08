package adr

import (
	"net/rpc"

	"github.com/brocaar/lorawan"
	"github.com/hashicorp/go-plugin"
)

// HandshakeConfig for ADR plugins.
var HandshakeConfig = plugin.HandshakeConfig{
	ProtocolVersion:  3,
	MagicCookieKey:   "ADR_PLUGIN",
	MagicCookieValue: "ADR_PLUGIN",
}

// Handler defines the ADR handler interface.
type Handler interface {
	ID() (string, error)
	Name() (string, error)
	Handle(HandleRequest) (HandleResponse, error)
}

// HandleRequest implements the ADR handle request.
type HandleRequest struct {
	// Region.
	Region string

	// DevEUI of the device.
	DevEUI lorawan.EUI64

	// MAC version of the device.
	MACVersion string

	// Regional parameter revision.
	RegParamsRevision string

	// ADR defines if the device has ADR enabled.
	ADR bool

	// DR holds the uplink data-rate of the device.
	DR int

	// TxPowerIndex holds the current tx-power index of the device.
	TxPowerIndex int

	// NbTrans holds the number of transmissions for the device.
	NbTrans int

	// MaxTxPowerIndex defines the max allowed tx-power index.
	MaxTxPowerIndex int

	// RequiredSNRForDR defines the min. required SNR for the current data-rate.
	RequiredSNRForDR float32

	// InstallationMargin defines the configured installation margin.
	InstallationMargin float32

	// MinDR defines the min. allowed data-rate.
	MinDR int

	// MaxDR defines the max. allowed data-rate.
	MaxDR int

	// UplinkHistory contains the meta-data of the last uplinks.
	// Note: this table is for the current data-date only!
	UplinkHistory []UplinkMetaData

	// MaxLoRaDR value computed based on the configured bands
	MaxLoRaDR int
}

// HandleResponse implements the ADR handle response.
type HandleResponse struct {
	// DR holds the data-rate to which the device must change.
	DR int

	// TxPowerIndex holds the tx-power index to which the device must change.
	TxPowerIndex int

	// NbTrans holds the number of transmissions which the device must use for each uplink.
	NbTrans int
}

// UplinkMetaData contains the meta-data of an uplink transmission.
type UplinkMetaData struct {
	FCnt         uint32
	MaxSNR       float32
	MaxRSSI      int32
	TXPowerIndex int
	GatewayCount int
}

// HandlerRPCServer implements the RPC server for the Handler interface.
type HandlerRPCServer struct {
	// Impl holds the interface implementation.
	Impl Handler
}

func (s *HandlerRPCServer) ID(req interface{}, resp *string) error {
	var err error
	*resp, err = s.Impl.ID()
	return err
}

func (s *HandlerRPCServer) Name(req interface{}, resp *string) error {
	var err error
	*resp, err = s.Impl.Name()
	return err
}

func (s *HandlerRPCServer) Handle(req HandleRequest, resp *HandleResponse) error {
	var err error
	*resp, err = s.Impl.Handle(req)
	return err
}

// HandlerRPC implements the RPC client for the Handler interface.
type HandlerRPC struct {
	client *rpc.Client
}

func (r *HandlerRPC) ID() (string, error) {
	var resp string
	err := r.client.Call("Plugin.ID", new(interface{}), &resp)
	return resp, err
}

func (r *HandlerRPC) Name() (string, error) {
	var resp string
	err := r.client.Call("Plugin.Name", new(interface{}), &resp)
	return resp, err
}

func (r *HandlerRPC) Handle(req HandleRequest) (HandleResponse, error) {
	var resp HandleResponse
	err := r.client.Call("Plugin.Handle", req, &resp)
	return resp, err
}

// HandlerPlugin implements plugin.Plugin.
type HandlerPlugin struct {
	// Impl holds the interface implementation.
	Impl Handler
}

func (p *HandlerPlugin) Server(*plugin.MuxBroker) (interface{}, error) {
	return &HandlerRPCServer{Impl: p.Impl}, nil
}

func (p *HandlerPlugin) Client(b *plugin.MuxBroker, c *rpc.Client) (interface{}, error) {
	return &HandlerRPC{client: c}, nil
}
