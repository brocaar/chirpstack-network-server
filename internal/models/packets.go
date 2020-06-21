package models

import (
	"github.com/brocaar/chirpstack-api/go/v3/gw"
	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/backend"
)

// RXPacket contains a received PHYPayload together with its RX metadata.
type RXPacket struct {
	// DR contains the data-rate index of the uplink.
	DR int

	// PHYPayload holds the uplink PHYPayload object.
	PHYPayload lorawan.PHYPayload

	// TXInfo holds the TX meta-data struct.
	TXInfo *gw.UplinkTXInfo

	// RXInfoSet holds all the RX meta-data elements of the receiving gateways.
	RXInfoSet []*gw.UplinkRXInfo

	// XmitDataReqPayload holds the uplink xmit data request payload in case
	// the uplink was received through a roaming network-server.
	XmitDataReqPayload *backend.XmitDataReqPayload
}
