package models

import (
	"time"

	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/band"
)

// RXPacket contains the PHYPayload received from GatewayMAC.
type RXPacket struct {
	RXInfo     RXInfo             `json:"rxInfo"`
	PHYPayload lorawan.PHYPayload `json:"phyPayload"`
}

// RXInfo contains the RX information.
type RXInfo struct {
	MAC       lorawan.EUI64 `json:"mac"`       // MAC address of the gateway
	Time      time.Time     `json:"time"`      // receive time
	Timestamp uint32        `json:"timestamp"` // gateway internal receive timestamp with microsecond precision, will rollover every ~ 72 minutes
	Frequency int           `json:"frequency"` // frequency in Hz
	Channel   int           `json:"channel"`   // concentrator IF channel used for RX
	RFChain   int           `json:"rfChain"`   // RF chain used for RX
	CRCStatus int           `json:"crcStatus"` // 1 = OK, -1 = fail, 0 = no CRC
	CodeRate  string        `json:"codeRate"`  // ECC code rate
	RSSI      int           `json:"rssi"`      // RSSI in dBm
	LoRaSNR   float64       `json:"loRaSNR"`   // LoRa signal-to-noise ratio in dB
	Size      int           `json:"size"`      // packet payload size
	DataRate  band.DataRate `json:"dataRate"`  // RX datarate (either LoRa or FSK)
}

// TXPacket contains the PHYPayload which should be send to the
// gateway.
type TXPacket struct {
	TXInfo     TXInfo             `json:"txInfo"`
	PHYPayload lorawan.PHYPayload `json:"phyPayload"`
}

// TXInfo contains the information used for TX.
type TXInfo struct {
	MAC         lorawan.EUI64 `json:"mac"`         // MAC address of the gateway
	Immediately bool          `json:"immediately"` // send the packet immediately (ignore Time)
	Timestamp   uint32        `json:"timestamp"`   // gateway internal receive timestamp with microsecond precision, will rollover every ~ 72 minutes
	Frequency   int           `json:"frequency"`   // frequency in Hz
	Power       int           `json:"power"`       // TX power to use in dBm
	DataRate    band.DataRate `json:"dataRate"`    // TX datarate (either LoRa or FSK)
	CodeRate    string        `json:"codeRate"`    // ECC code rate
}

// GatewayStatsPacket contains the information of a gateway.
type GatewayStatsPacket struct {
	MAC                 lorawan.EUI64 `json:"mac"`
	Time                time.Time     `json:"time"`
	Latitude            float64       `json:"latitude"`
	Longitude           float64       `json:"longitude"`
	Altitude            float64       `json:"altitude"`
	RXPacketsReceived   int           `json:"rxPacketsReceived"`
	RXPacketsReceivedOK int           `json:"rxPacketsReceivedOK"`
}

// RXPayload contains the received (decrypted) payload from the node
// which will be sent to the application.
type RXPayload struct {
	DevEUI       lorawan.EUI64 `json:"devEUI"`
	FPort        uint8         `json:"fPort"`
	GatewayCount int           `json:"gatewayCount"`
	RSSI         int           `json:"rssi"`
	Data         []byte        `json:"data"`
}

// TXPayload contains the payload to be sent to the node.
type TXPayload struct {
	Reference string        `json:"reference"` // external reference that needs to be defined by the application, used for error notifications
	Confirmed bool          `json:"confirmed"` // indicates if the packet needs to be confirmed with an ACK
	DevEUI    lorawan.EUI64 `json:"devEUI"`    // the DevEUI of the node to send the payload to
	FPort     uint8         `json:"fPort"`     // the FPort
	Data      []byte        `json:"data"`      // the data to send (unencrypted, in JSON this must be the base64 representation of the data)
}

// MACPayload contains data from a MAC command.
type MACPayload struct {
	Reference  string        `json:"reference,omitempty"` // external reference that needs to be defined by the network-controller
	DevEUI     lorawan.EUI64 `json:"devEUI"`
	MACCommand []byte        `json:"macCommand"`
}

// RXInfoPayload defines the payload containing the RX information sent to
// the network-controller on each received packet.
type RXInfoPayload struct {
	DevEUI lorawan.EUI64 `json:"devEUI"`
	ADR    bool          `json:"adr"`
	FCnt   uint32        `json:"fCnt"`
	RXInfo []RXInfo      `json:"rxInfo"`
}
