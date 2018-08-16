package gw

import (
	"errors"
	"time"

	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/band"
)

// RXPacket contains the PHYPayload received from the gateway.
type RXPacket struct {
	RXInfo     RXInfo             `json:"rxInfo"`
	PHYPayload lorawan.PHYPayload `json:"phyPayload"`
}

// RXPacketBytes contains the PHYPayload as []byte received from the gateway.
type RXPacketBytes struct {
	RXInfo     RXInfo `json:"rxInfo"`
	PHYPayload []byte `json:"phyPayload"`
}

// RXInfo contains the RX information.
type RXInfo struct {
	MAC               lorawan.EUI64 `json:"mac"`                         // MAC address of the gateway
	Time              *time.Time    `json:"time,omitempty"`              // Receive timestamp (only set when the gateway has a GPS time-source)
	TimeSinceGPSEpoch *Duration     `json:"timeSinceGPSEpoch,omitempty"` // Time since GPS epoch (1980-01-06, only set when the gateway has a GPS time source)
	Timestamp         uint32        `json:"timestamp"`                   // gateway internal receive timestamp with microsecond precision, will rollover every ~ 72 minutes
	Frequency         int           `json:"frequency"`                   // frequency in Hz
	Channel           int           `json:"channel"`                     // concentrator IF channel used for RX
	RFChain           int           `json:"rfChain"`                     // RF chain used for RX
	CRCStatus         int           `json:"crcStatus"`                   // 1 = OK, -1 = fail, 0 = no CRC
	CodeRate          string        `json:"codeRate"`                    // ECC code rate
	RSSI              int           `json:"rssi"`                        // RSSI in dBm
	LoRaSNR           float64       `json:"loRaSNR"`                     // LoRa signal-to-noise ratio in dB
	Size              int           `json:"size"`                        // packet payload size
	DataRate          band.DataRate `json:"dataRate"`                    // RX datarate (either LoRa or FSK)
	Board             int           `json:"board"`                       // Concentrator board used for RX
	Antenna           int           `json:"antenna"`                     // Antenna number on which signal has been received
}

// TXPacket contains the PHYPayload which should be send to the
// gateway.
type TXPacket struct {
	Token      uint16             `json:"token"`
	TXInfo     TXInfo             `json:"txInfo"`
	PHYPayload lorawan.PHYPayload `json:"phyPayload"`
}

// TXPacketBytes contains the PHYPayload as []byte which should be send to the
// gateway.
type TXPacketBytes struct {
	Token      uint16 `json:"token"`
	TXInfo     TXInfo `json:"txInfo"`
	PHYPayload []byte `json:"phyPayload"`
}

// Duration implements time.Duration, but encoded to json, uses the String
// formatting.
type Duration time.Duration

// MarshalJSON implements the json.Marshaler interface.
func (d Duration) MarshalJSON() ([]byte, error) {
	return []byte(`"` + time.Duration(d).String() + `"`), nil
}

// UnmarshalJSON implements the json.Unmarshaler interface.
func (d *Duration) UnmarshalJSON(data []byte) error {
	if len(data) < 2 {
		return errors.New("invalid duration string")
	}
	data = data[1 : len(data)-1]

	du, err := time.ParseDuration(string(data))
	if err != nil {
		return err
	}
	*d = Duration(du)
	return nil
}

// TXInfo contains the information used for TX.
type TXInfo struct {
	MAC               lorawan.EUI64 `json:"mac"`                         // MAC address of the gateway
	Immediately       bool          `json:"immediately"`                 // send the packet immediately (ignore Time)
	TimeSinceGPSEpoch *Duration     `json:"timeSinceGPSEpoch,omitempty"` // Transmit at time since GPS epoch (since 1980-01-06, only possible when the gateway has a GPS time source)
	Timestamp         *uint32       `json:"timestamp,omitempty"`         // transmit at gateway internal timestamp (microsecond precision, will rollover every ~ 72 minutes)
	Frequency         int           `json:"frequency"`                   // frequency in Hz
	Power             int           `json:"power"`                       // TX power to use in dBm
	DataRate          band.DataRate `json:"dataRate"`                    // TX datarate (either LoRa or FSK)
	CodeRate          string        `json:"codeRate"`                    // ECC code rate
	IPol              *bool         `json:"iPol"`                        // when left nil, the gateway-bridge will use the default (true for LoRa modulation)
	Board             int           `json:"board"`                       // Concentrator board used for RX
	Antenna           int           `json:"antenna"`                     // Antenna number on which signal has been received
}

// GatewayStatsPacket contains the information of a gateway.
type GatewayStatsPacket struct {
	MAC                 lorawan.EUI64          `json:"mac"`
	Time                time.Time              `json:"time,omitempty"`
	Latitude            *float64               `json:"latitude,omitempty"`
	Longitude           *float64               `json:"longitude,omitempty"`
	Altitude            *float64               `json:"altitude,omitempty"`
	RXPacketsReceived   int                    `json:"rxPacketsReceived"`
	RXPacketsReceivedOK int                    `json:"rxPacketsReceivedOK"`
	TXPacketsReceived   int                    `json:"txPacketsReceived"`
	TXPacketsEmitted    int                    `json:"txPacketsEmitted"`
	ConfigVersion       string                 `json:"configVersion,omitempty"`
	CustomData          map[string]interface{} `json:"customData,omitempty"` // custom fields defined by alternative packet_forwarder versions (e.g. TTN sends platform, contactEmail, and description)
}

// Possible TX errors.
const (
	ErrTooLate          = "TOO_LATE"
	ErrTooEarly         = "TOO_EARLY"
	ErrCollisionPacket  = "COLLISION_PACKET"
	ErrCollisionBeackon = "COLLISION_BEACON"
	ErrTXFrequency      = "TX_FREQ"
	ErrTXPower          = "TX_POWER"
	ErrGPSUnlocked      = "GPS_UNLOCKED"
)

// TXAck contains the acknowledgement of the requested frame transmission
// or an error.
type TXAck struct {
	MAC   lorawan.EUI64 `json:"mac"`
	Token uint16        `json:"token"`
	Error string        `json:"error,omitempty"`
}

// GatewayConfigPacket contains the configuration to be pushed
// to the gateway.
type GatewayConfigPacket struct {
	MAC      lorawan.EUI64 `json:"mac"`
	Version  string        `json:"version"`
	Channels []Channel     `json:"channels"`
}

// Channel defines a gateay channel.
type Channel struct {
	Modulation       band.Modulation `json:"modulation"`
	Frequency        int             `json:"frequency"`
	Bandwidth        int             `json:"bandwidth"`
	Bitrate          int             `json:"bitrate,omitempty"`          // FSK modulation only
	SpreadingFactors []int           `json:"spreadingFactors,omitempty"` // LoRa modulation only
}
