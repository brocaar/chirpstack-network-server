package loraserver

import (
	"time"

	"github.com/brocaar/lorawan"
)

// DataRate types
const (
	DataRateLoRa = "LORA"
	DataRateFSK  = "FSK"
)

// DataRate contains either the LoRa datarate identifier or the FSK datarate.
type DataRate struct {
	LoRa string // LoRa datarate identifier (e.g. SF12BW500)  OR
	FSK  uint   // FSK datarate (the frame's bit rate in Hz)
}

// Modulation returns the modulation type.
func (r DataRate) Modulation() string {
	if r.LoRa != "" {
		return DataRateLoRa
	}
	return DataRateFSK
}

// RXPackets is a slice of RXPacket. It implements sort.Interface
// to sort the slice of packets by signal strength so that the
// packet received with the strongest signal will be at index 0
// of RXPackets.
type RXPackets []RXPacket

// Len is part of sort.Interface.
func (p RXPackets) Len() int {
	return len(p)
}

// Swap is part of sort.Interface.
func (p RXPackets) Swap(i, j int) {
	p[i], p[j] = p[j], p[i]
}

// Less is part of sort.Interface.
func (p RXPackets) Less(i, j int) bool {
	return p[i].RXInfo.RSSI > p[j].RXInfo.RSSI
}

// RXPacket contains the PHYPayload received from GatewayMAC.
type RXPacket struct {
	RXInfo     RXInfo
	PHYPayload lorawan.PHYPayload
}

// RXInfo contains the RX information.
type RXInfo struct {
	MAC        lorawan.EUI64 // MAC address of the gateway
	Time       time.Time     // receive time
	Timestamp  uint32        // gateway internal receive timestamp with microsecond precision, will rollover every ~ 72 minutes
	Frequency  float64       // frequency in Mhz
	Channel    uint          // concentrator IF channel used for RX
	RFChain    uint          // RF chain used for RX
	CRCStatus  int           // 1 = OK, -1 = fail, 0 = no CRC
	Modulation string        // "LORA" or "FSK"
	CodeRate   string        // ECC code rate
	RSSI       int           // RSSI in dBm
	LoRaSNR    float64       // LoRa signal-to-noise ratio in dB
	Size       uint          // packet payload size
	DataRate   DataRate      // RX datarate (either LoRa or FSK)
}

// TXPacket contains the PHYPayload which should be send to the
// gateway.
type TXPacket struct {
	TXInfo     TXInfo
	PHYPayload lorawan.PHYPayload
}

// TXInfo contains the information used for TX.
type TXInfo struct {
	MAC                lorawan.EUI64 // MAC address of the gateway
	Immediately        bool          // send the packet immediately (ignore Time)
	Timestamp          uint32        // gateway internal receive timestamp with microsecond precision, will rollover every ~ 72 minutes
	Frequency          float64       // frequency in MHz
	RFChain            uint          // RF chain to use for TX
	Power              uint          // TX power to use in dBm
	DataRate           DataRate      // TX datarate (either LoRa or FSK)
	CodeRate           string        // ECC code rate
	FrequencyDeviation uint          // FSK frequency deviation (unsigned integer, in Hz)
	DisableCRC         bool          // disable the CRC of the physical layer
}

// GatewayStatsPacket contains the information of a gateway.
type GatewayStatsPacket struct {
	MAC                 lorawan.EUI64
	Time                time.Time
	Latitude            float64
	Longitude           float64
	Altitude            float64
	RXPacketsReceived   int
	RXPacketsReceivedOK int
}
