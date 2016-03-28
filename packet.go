package loraserver

import (
	"bytes"
	"encoding/gob"
	"encoding/hex"
	"errors"
	"sort"
	"time"

	"github.com/brocaar/lorawan"
	"github.com/garyburd/redigo/redis"
)

// Packet collection constants
const (
	CollectAndCallOnceWait = time.Millisecond * 100 // the time to wait for the same packet received by multiple gateways
	CollectDataDownWait    = time.Millisecond * 100 // the time to wait on possible downlink payloads from the application
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

// RXPayload contains the received (decrypted) payload from the node
// which will be sent to the application.
type RXPayload struct {
	DevEUI       lorawan.EUI64 `json:"devEUI"`
	FPort        uint8         `json:"fPort"`
	GatewayCount int           `json:"gatewayCount"`
	Data         []byte        `json:"data"`
}

// TXPayload contains the payload to be sent to the node.
type TXPayload struct {
	Confirmed bool          `json:"confirmed"` // indicates if the packet needs to be confirmed with an ACK
	DevEUI    lorawan.EUI64 `json:"devEUI"`    // the DevEUI of the node to send the payload to
	FPort     uint8         `json:"fPort"`     // the FPort
	Data      []byte        `json:"data"`      // the data to send (unencrypted, in JSON this must be the base64 representation of the data)
}

// collectAndCallOnce collects the package, sleeps the configured duraction and
// calls the callback only once with a slice of packets, sorted by signal
// strength (strongest at index 0). This method exists since multiple gateways
// are able to receive the same packet, but the packet needs to processed
// only once.
// It is important to validate the packet before calling collectAndCallOnce
// (since the MIC is part of the storage key, make sure it is valid).
// It is safe to collect the same packet received by the same gateway twice.
// Since the underlying storage type is a set, the result will always be a
// unique set per gateway MAC and packet MIC.
func collectAndCallOnce(p *redis.Pool, rxPacket RXPacket, callback func(packets RXPackets) error) error {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(rxPacket); err != nil {
		return err
	}
	c := p.Get()
	defer c.Close()

	// store the packet in a set with CollectAndCallOnceWait expiration
	// in case the packet is received by multiple gateways, the set will contain
	// each packet.
	key := "collect_" + hex.EncodeToString(rxPacket.PHYPayload.MIC[:])
	c.Send("MULTI")
	c.Send("SADD", key, buf.Bytes())
	c.Send("PEXPIRE", key, int64(CollectAndCallOnceWait*2)/int64(time.Millisecond))
	_, err := c.Do("EXEC")
	if err != nil {
		return err
	}

	// acquire a lock on processing this packet
	_, err = redis.String((c.Do("SET", key+"_lock", "lock", "PX", int64(CollectAndCallOnceWait*2)/int64(time.Millisecond), "NX")))
	if err != nil {
		if err == redis.ErrNil {
			// the packet processing is already locked by an other process
			// so there is nothing to do anymore :-)
			return nil
		}
		return err
	}

	// wait the configured amount of time, more packets might be received
	// from other gateways
	time.Sleep(CollectAndCallOnceWait)

	// collect all packets from the set
	packets := make(RXPackets, 0)
	payloads, err := redis.ByteSlices(c.Do("SMEMBERS", key))
	if err != nil {
		return err
	}
	if len(payloads) == 0 {
		return errors.New("the collected packets set returned zero items")
	}

	for _, b := range payloads {
		var packet RXPacket
		if err := gob.NewDecoder(bytes.NewReader(b)).Decode(&packet); err != nil {
			return err
		}
		packets = append(packets, packet)
	}

	sort.Sort(packets)
	return callback(packets)
}
