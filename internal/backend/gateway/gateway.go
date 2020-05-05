package gateway

import (
	"errors"
	"fmt"

	"github.com/brocaar/chirpstack-api/go/v3/gw"
	"github.com/golang/protobuf/proto"
)

var backend Gateway

// Backend returns the gateway backend.
func Backend() Gateway {
	return backend
}

// SetBackend sets the given gateway backend.
func SetBackend(b Gateway) {
	backend = b
}

// Gateway is the interface of a gateway backend.
// A gateway backend is responsible for the communication with the gateway.
type Gateway interface {
	SendTXPacket(gw.DownlinkFrame) error                   // send the given packet to the gateway
	SendGatewayConfigPacket(gw.GatewayConfiguration) error // SendGatewayConfigPacket sends the given GatewayConfigPacket to the gateway.
	RXPacketChan() chan gw.UplinkFrame                     // channel containing the received packets
	StatsPacketChan() chan gw.GatewayStats                 // channel containing the received gateway stats
	DownlinkTXAckChan() chan gw.DownlinkTXAck              // channel containing the downlink tx acknowledgements
	Close() error                                          // close the gateway backend.
}

// UpdateDownlinkFrame updates the downlink frame for backward compatibility.
func UpdateDownlinkFrame(mode string, df *gw.DownlinkFrame) error {
	if len(df.Items) == 0 {
		return errors.New("items must contain at least one item")
	}

	switch mode {
	case "hybrid":
		df.PhyPayload = df.Items[0].PhyPayload
		df.TxInfo = proto.Clone(df.Items[0].TxInfo).(*gw.DownlinkTXInfo)
		df.TxInfo.GatewayId = df.GatewayId
	case "legacy":
		df.PhyPayload = df.Items[0].PhyPayload
		df.TxInfo = proto.Clone(df.Items[0].TxInfo).(*gw.DownlinkTXInfo)
		df.TxInfo.GatewayId = df.GatewayId
		df.Items = nil
	case "multi_only":
		// this is internally the default
		return nil
	default:
		return fmt.Errorf("invalid multi_downlink_feature setting: %s", mode)
	}

	return nil
}
