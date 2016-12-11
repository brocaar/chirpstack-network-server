package lorawan

import (
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
)

// macPayloadMutex is used when registering proprietary MAC command payloads to
// the macPayloadRegistry.
var macPayloadMutex sync.RWMutex

// CID defines the MAC command identifier.
type CID byte

// MAC commands as specified by the LoRaWAN R1.0 specs. Note that each *Req / *Ans
// has the same value. Based on the fact if a message is uplink or downlink
// you should use on or the other.
const (
	LinkCheckReq     CID = 0x02
	LinkCheckAns     CID = 0x02
	LinkADRReq       CID = 0x03
	LinkADRAns       CID = 0x03
	DutyCycleReq     CID = 0x04
	DutyCycleAns     CID = 0x04
	RXParamSetupReq  CID = 0x05
	RXParamSetupAns  CID = 0x05
	DevStatusReq     CID = 0x06
	DevStatusAns     CID = 0x06
	NewChannelReq    CID = 0x07
	NewChannelAns    CID = 0x07
	RXTimingSetupReq CID = 0x08
	RXTimingSetupAns CID = 0x08
	TXParamSetupReq  CID = 0x09
	TXParamSetupAns  CID = 0x09
	DLChannelReq     CID = 0x0A
	DLChannelAns     CID = 0x0A
	// 0x80 to 0xFF reserved for proprietary network command extensions
)

// macPayloadInfo contains the info about a MAC payload
type macPayloadInfo struct {
	size    int
	payload func() MACCommandPayload
}

// macPayloadRegistry contains the info for uplink and downlink MAC payloads
// in the format map[uplink]map[CID].
// Note that MAC command that do not have a payload are not included in this
// list.
var macPayloadRegistry = map[bool]map[CID]macPayloadInfo{
	false: map[CID]macPayloadInfo{
		LinkCheckAns:     {2, func() MACCommandPayload { return &LinkCheckAnsPayload{} }},
		LinkADRReq:       {4, func() MACCommandPayload { return &LinkADRReqPayload{} }},
		DutyCycleReq:     {1, func() MACCommandPayload { return &DutyCycleReqPayload{} }},
		RXParamSetupReq:  {4, func() MACCommandPayload { return &RX2SetupReqPayload{} }},
		NewChannelReq:    {5, func() MACCommandPayload { return &NewChannelReqPayload{} }},
		RXTimingSetupReq: {1, func() MACCommandPayload { return &RXTimingSetupReqPayload{} }},
		TXParamSetupReq:  {1, func() MACCommandPayload { return &TXParamSetupReqPayload{} }},
		DLChannelReq:     {4, func() MACCommandPayload { return &DLChannelReqPayload{} }},
	},
	true: map[CID]macPayloadInfo{
		LinkADRAns:      {1, func() MACCommandPayload { return &LinkADRAnsPayload{} }},
		RXParamSetupAns: {1, func() MACCommandPayload { return &RX2SetupAnsPayload{} }},
		DevStatusAns:    {2, func() MACCommandPayload { return &DevStatusAnsPayload{} }},
		NewChannelAns:   {1, func() MACCommandPayload { return &NewChannelAnsPayload{} }},
		DLChannelAns:    {1, func() MACCommandPayload { return &DLChannelAnsPayload{} }},
	},
}

// DwellTime defines the dwell time type.
type DwellTime int

// Possible dwell time options.
const (
	DwellTimeNoLimit DwellTime = iota
	DwellTime400ms
)

// GetMACPayloadAndSize returns a new MACCommandPayload instance and it's size.
func GetMACPayloadAndSize(uplink bool, c CID) (MACCommandPayload, int, error) {
	macPayloadMutex.RLock()
	defer macPayloadMutex.RUnlock()

	v, ok := macPayloadRegistry[uplink][c]
	if !ok {
		return nil, 0, fmt.Errorf("lorawan: payload unknown for uplink=%v and CID=%v", uplink, c)
	}

	return v.payload(), v.size, nil
}

// RegisterProprietaryMACCommand registers a proprietary MAC command. Note
// that there is no need to call this when the size of the payload is > 0 bytes.
func RegisterProprietaryMACCommand(uplink bool, cid CID, payloadSize int) error {
	if !(cid >= 128 && cid <= 255) {
		return fmt.Errorf("lorawan: invalid CID %x", cid)
	}

	if payloadSize == 0 {
		// no need to register the payload size
		return nil
	}

	macPayloadMutex.Lock()
	defer macPayloadMutex.Unlock()

	macPayloadRegistry[uplink][cid] = macPayloadInfo{
		size:    payloadSize,
		payload: func() MACCommandPayload { return &ProprietaryMACCommandPayload{} },
	}

	return nil
}

// MACCommandPayload is the interface that every MACCommand payload
// must implement.
type MACCommandPayload interface {
	MarshalBinary() (data []byte, err error)
	UnmarshalBinary(data []byte) error
}

// MACCommand represents a MAC command with optional payload.
type MACCommand struct {
	CID     CID
	Payload MACCommandPayload
}

// MarshalBinary marshals the object in binary form.
func (m MACCommand) MarshalBinary() ([]byte, error) {
	if !(m.CID >= 2 && m.CID <= 8) && !(m.CID >= 128) {
		return nil, fmt.Errorf("lorawan: invalid CID %x", m.CID)
	}

	b := []byte{byte(m.CID)}
	if m.Payload != nil {
		p, err := m.Payload.MarshalBinary()
		if err != nil {
			return nil, err
		}
		b = append(b, p...)
	}
	return b, nil
}

// UnmarshalBinary decodes the object from binary form.
func (m *MACCommand) UnmarshalBinary(uplink bool, data []byte) error {
	if len(data) == 0 {
		return errors.New("lorawan: at least 1 byte of data is expected")
	}

	m.CID = CID(data[0])
	if !(m.CID >= 2 && m.CID <= 8) && !(m.CID >= 128) {
		return fmt.Errorf("lorawan: invalid CID %x", m.CID)
	}

	if len(data) > 1 {
		p, _, err := GetMACPayloadAndSize(uplink, m.CID)
		if err != nil {
			return err
		}
		m.Payload = p
		if err := m.Payload.UnmarshalBinary(data[1:]); err != nil {
			return err
		}
	}
	return nil
}

// ProprietaryMACCommandPayload represents a proprietary payload.
type ProprietaryMACCommandPayload struct {
	Bytes []byte
}

// MarshalBinary marshals the object into a slice of bytes.
func (p ProprietaryMACCommandPayload) MarshalBinary() ([]byte, error) {
	return p.Bytes, nil
}

// UnmarshalBinary decodes the object from a slice of bytes.
func (p *ProprietaryMACCommandPayload) UnmarshalBinary(data []byte) error {
	p.Bytes = data
	return nil
}

// LinkCheckAnsPayload represents the LinkCheckAns payload.
type LinkCheckAnsPayload struct {
	Margin uint8
	GwCnt  uint8
}

// MarshalBinary marshals the object in binary form.
func (p LinkCheckAnsPayload) MarshalBinary() ([]byte, error) {
	return []byte{byte(p.Margin), byte(p.GwCnt)}, nil
}

// UnmarshalBinary decodes the object from binary form.
func (p *LinkCheckAnsPayload) UnmarshalBinary(data []byte) error {
	if len(data) != 2 {
		return errors.New("lorawan: 2 bytes of data are expected")
	}
	p.Margin = uint8(data[0])
	p.GwCnt = uint8(data[1])
	return nil
}

// ChMask encodes the channels usable for uplink access. 0 = channel 1,
// 15 = channel 16.
type ChMask [16]bool

// MarshalBinary marshals the object in binary form.
func (m ChMask) MarshalBinary() ([]byte, error) {
	b := make([]byte, 2)
	for i := uint8(0); i < 16; i++ {
		if m[i] {
			b[i/8] = b[i/8] ^ 1<<(i%8)
		}
	}
	return b, nil
}

// UnmarshalBinary decodes the object from binary form.
func (m *ChMask) UnmarshalBinary(data []byte) error {
	if len(data) != 2 {
		return errors.New("lorawan: 2 bytes of data are expected")
	}
	for i, b := range data {
		for j := uint8(0); j < 8; j++ {
			if b&(1<<j) > 0 {
				m[uint8(i)*8+j] = true
			}
		}
	}
	return nil
}

// Redundancy represents the redundancy field.
type Redundancy struct {
	ChMaskCntl uint8
	NbRep      uint8
}

// MarshalBinary marshals the object in binary form.
func (r Redundancy) MarshalBinary() ([]byte, error) {
	b := make([]byte, 1)
	if r.NbRep > 15 {
		return b, errors.New("lorawan: max value of NbRep is 15")
	}
	if r.ChMaskCntl > 7 {
		return b, errors.New("lorawan: max value of ChMaskCntl is 7")
	}
	b[0] = r.NbRep ^ (r.ChMaskCntl << 4)
	return b, nil
}

// UnmarshalBinary decodes the object from binary form.
func (r *Redundancy) UnmarshalBinary(data []byte) error {
	if len(data) != 1 {
		return errors.New("lorawan: 1 byte of data is expected")
	}
	r.NbRep = data[0] & ((1 << 3) ^ (1 << 2) ^ (1 << 1) ^ (1 << 0))
	r.ChMaskCntl = (data[0] & ((1 << 6) ^ (1 << 5) ^ (1 << 4))) >> 4
	return nil
}

// LinkADRReqPayload represents the LinkADRReq payload.
type LinkADRReqPayload struct {
	DataRate   uint8
	TXPower    uint8
	ChMask     ChMask
	Redundancy Redundancy
}

// MarshalBinary marshals the object in binary form.
func (p LinkADRReqPayload) MarshalBinary() ([]byte, error) {
	b := make([]byte, 0, 4)
	if p.DataRate > 15 {
		return b, errors.New("lorawan: the max value of DataRate is 15")
	}
	if p.TXPower > 15 {
		return b, errors.New("lorawan: the max value of TXPower is 15")
	}

	cm, err := p.ChMask.MarshalBinary()
	if err != nil {
		return b, err
	}
	r, err := p.Redundancy.MarshalBinary()
	if err != nil {
		return b, err
	}

	b = append(b, p.TXPower^(p.DataRate<<4))
	b = append(b, cm...)
	b = append(b, r...)

	return b, nil
}

// UnmarshalBinary decodes the object from binary form.
func (p *LinkADRReqPayload) UnmarshalBinary(data []byte) error {
	if len(data) != 4 {
		return errors.New("lorawan: 4 bytes of data are expected")
	}
	p.DataRate = (data[0] & ((1 << 7) ^ (1 << 6) ^ (1 << 5) ^ (1 << 4))) >> 4
	p.TXPower = data[0] & ((1 << 3) ^ (1 << 2) ^ (1 << 1) ^ (1 << 0))

	if err := p.ChMask.UnmarshalBinary(data[1:3]); err != nil {
		return err
	}
	if err := p.Redundancy.UnmarshalBinary(data[3:4]); err != nil {
		return err
	}
	return nil
}

// LinkADRAnsPayload represents the LinkADRAns payload.
type LinkADRAnsPayload struct {
	ChannelMaskACK bool
	DataRateACK    bool
	PowerACK       bool
}

// MarshalBinary marshals the object in binary form.
func (p LinkADRAnsPayload) MarshalBinary() ([]byte, error) {
	var b byte
	if p.ChannelMaskACK {
		b = b ^ (1 << 0)
	}
	if p.DataRateACK {
		b = b ^ (1 << 1)
	}
	if p.PowerACK {
		b = b ^ (1 << 2)
	}
	return []byte{b}, nil
}

// UnmarshalBinary decodes the object from binary form.
func (p *LinkADRAnsPayload) UnmarshalBinary(data []byte) error {
	if len(data) != 1 {
		return errors.New("lorawan: 1 byte of data is expected")
	}
	if data[0]&(1<<0) > 0 {
		p.ChannelMaskACK = true
	}
	if data[0]&(1<<1) > 0 {
		p.DataRateACK = true
	}
	if data[0]&(1<<2) > 0 {
		p.PowerACK = true
	}
	return nil
}

// DutyCycleReqPayload represents the DutyCycleReq payload.
type DutyCycleReqPayload struct {
	MaxDCCycle uint8
}

// MarshalBinary marshals the object in binary form.
func (p DutyCycleReqPayload) MarshalBinary() ([]byte, error) {
	b := make([]byte, 0, 1)
	if p.MaxDCCycle > 15 && p.MaxDCCycle < 255 {
		return b, errors.New("lorawan: only a MaxDCycle value of 0 - 15 and 255 is allowed")
	}
	b = append(b, p.MaxDCCycle)
	return b, nil
}

// UnmarshalBinary decodes the object from binary form.
func (p *DutyCycleReqPayload) UnmarshalBinary(data []byte) error {
	if len(data) != 1 {
		return errors.New("lorawan: 1 byte of data is expected")
	}
	p.MaxDCCycle = data[0]
	return nil
}

// DLSettings represents the DLSettings fields (downlink settings).
type DLSettings struct {
	RX2DataRate uint8
	RX1DROffset uint8
}

// MarshalBinary marshals the object in binary form.
func (s DLSettings) MarshalBinary() ([]byte, error) {
	b := make([]byte, 0, 1)
	if s.RX2DataRate > 15 {
		return b, errors.New("lorawan: max value of RX2DataRate is 15")
	}
	if s.RX1DROffset > 7 {
		return b, errors.New("lorawan: max value of RX1DROffset is 7")
	}
	b = append(b, s.RX2DataRate^(s.RX1DROffset<<4))
	return b, nil
}

// UnmarshalBinary decodes the object from binary form.
func (s *DLSettings) UnmarshalBinary(data []byte) error {
	if len(data) != 1 {
		return errors.New("lorawan: 1 byte of data is expected")
	}
	s.RX2DataRate = data[0] & ((1 << 3) ^ (1 << 2) ^ (1 << 1) ^ (1 << 0))
	s.RX1DROffset = (data[0] & ((1 << 6) ^ (1 << 5) ^ (1 << 4))) >> 4
	return nil
}

// RX2SetupReqPayload represents the RX2SetupReq payload.
type RX2SetupReqPayload struct {
	Frequency  uint32
	DLSettings DLSettings
}

// MarshalBinary marshals the object in binary form.
func (p RX2SetupReqPayload) MarshalBinary() ([]byte, error) {
	b := make([]byte, 5)
	if p.Frequency/100 >= 16777216 { // 2^24
		return b, errors.New("lorawan: max value of Frequency is 2^24-1")
	}
	if p.Frequency%100 != 0 {
		return b, errors.New("lorawan: Frequency must be a multiple of 100")
	}
	bytes, err := p.DLSettings.MarshalBinary()
	if err != nil {
		return b, err
	}
	b[0] = bytes[0]

	binary.LittleEndian.PutUint32(b[1:5], p.Frequency/100)
	// we don't return the last octet which is fine since we're only interested
	// in the 24 LSB of Frequency
	return b[0:4], nil
}

// UnmarshalBinary decodes the object from binary form.
func (p *RX2SetupReqPayload) UnmarshalBinary(data []byte) error {
	if len(data) != 4 {
		return errors.New("lorawan: 4 bytes of data are expected")
	}
	if err := p.DLSettings.UnmarshalBinary(data[0:1]); err != nil {
		return err
	}
	// append one block of empty bits at the end of the slice since the
	// binary to uint32 expects 32 bits.
	b := make([]byte, len(data))
	copy(b, data)
	b = append(b, byte(0))
	p.Frequency = binary.LittleEndian.Uint32(b[1:5]) * 100
	return nil
}

// RX2SetupAnsPayload represents the RX2SetupAns payload.
type RX2SetupAnsPayload struct {
	ChannelACK     bool
	RX2DataRateACK bool
	RX1DROffsetACK bool
}

// MarshalBinary marshals the object in binary form.
func (p RX2SetupAnsPayload) MarshalBinary() ([]byte, error) {
	var b byte
	if p.ChannelACK {
		b = b ^ (1 << 0)
	}
	if p.RX2DataRateACK {
		b = b ^ (1 << 1)
	}
	if p.RX1DROffsetACK {
		b = b ^ (1 << 2)
	}
	return []byte{b}, nil
}

// UnmarshalBinary decodes the object from binary form.
func (p *RX2SetupAnsPayload) UnmarshalBinary(data []byte) error {
	if len(data) != 1 {
		return errors.New("lorawan: 1 byte of data is expected")
	}
	p.ChannelACK = data[0]&(1<<0) > 0
	p.RX2DataRateACK = data[0]&(1<<1) > 0
	p.RX1DROffsetACK = data[0]&(1<<2) > 0
	return nil
}

// DevStatusAnsPayload represents the DevStatusAns payload.
type DevStatusAnsPayload struct {
	Battery uint8
	Margin  int8
}

// MarshalBinary marshals the object in binary form.
func (p DevStatusAnsPayload) MarshalBinary() ([]byte, error) {
	b := make([]byte, 0, 2)
	if p.Margin < -32 {
		return b, errors.New("lorawan: min value of Margin is -32")
	}
	if p.Margin > 31 {
		return b, errors.New("lorawan: max value of Margin is 31")
	}

	b = append(b, p.Battery)
	if p.Margin < 0 {
		b = append(b, uint8(64+p.Margin))
	} else {
		b = append(b, uint8(p.Margin))
	}
	return b, nil
}

// UnmarshalBinary decodes the object from binary form.
func (p *DevStatusAnsPayload) UnmarshalBinary(data []byte) error {
	if len(data) != 2 {
		return errors.New("lorawan: 2 bytes of data are expected")
	}
	p.Battery = data[0]
	if data[1] > 31 {
		p.Margin = int8(data[1]) - 64
	} else {
		p.Margin = int8(data[1])
	}
	return nil
}

// NewChannelReqPayload represents the NewChannelReq payload.
type NewChannelReqPayload struct {
	ChIndex uint8
	Freq    uint32
	MaxDR   uint8
	MinDR   uint8
}

// MarshalBinary marshals the object in binary form.
func (p NewChannelReqPayload) MarshalBinary() ([]byte, error) {
	b := make([]byte, 5)
	if p.Freq/100 >= 16777216 { // 2^24
		return b, errors.New("lorawan: max value of Freq is 2^24 - 1")
	}
	if p.Freq%100 != 0 {
		return b, errors.New("lorawan: Freq must be a multiple of 100")
	}
	if p.MaxDR > 15 {
		return b, errors.New("lorawan: max value of MaxDR is 15")
	}
	if p.MinDR > 15 {
		return b, errors.New("lorawan: max value of MinDR is 15")
	}

	// we're borrowing the last byte b[4] because PutUint32 needs 4 bytes,
	// the last byte b[4] will be set to 0 because max Freq = 2^24 - 1
	binary.LittleEndian.PutUint32(b[1:5], p.Freq/100)
	b[0] = p.ChIndex
	b[4] = p.MinDR ^ (p.MaxDR << 4)

	return b, nil
}

// UnmarshalBinary decodes the object from binary form.
func (p *NewChannelReqPayload) UnmarshalBinary(data []byte) error {
	if len(data) != 5 {
		return errors.New("lorawan: 5 bytes of data are expected")
	}
	p.ChIndex = data[0]
	p.MinDR = data[4] & ((1 << 3) ^ (1 << 2) ^ (1 << 1) ^ (1 << 0))
	p.MaxDR = (data[4] & ((1 << 7) ^ (1 << 6) ^ (1 << 5) ^ (1 << 4))) >> 4

	b := make([]byte, len(data))
	copy(b, data)
	b[4] = byte(0)
	p.Freq = binary.LittleEndian.Uint32(b[1:5]) * 100
	return nil
}

// NewChannelAnsPayload represents the NewChannelAns payload.
type NewChannelAnsPayload struct {
	ChannelFrequencyOK bool
	DataRateRangeOK    bool
}

// MarshalBinary marshals the object in binary form.
func (p NewChannelAnsPayload) MarshalBinary() ([]byte, error) {
	var b byte
	if p.ChannelFrequencyOK {
		b = (1 << 0)
	}
	if p.DataRateRangeOK {
		b = b ^ (1 << 1)
	}
	return []byte{b}, nil
}

// UnmarshalBinary decodes the object from binary form.
func (p *NewChannelAnsPayload) UnmarshalBinary(data []byte) error {
	if len(data) != 1 {
		return errors.New("lorawan: 1 byte of data is expected")
	}
	p.ChannelFrequencyOK = data[0]&(1<<0) > 0
	p.DataRateRangeOK = data[0]&(1<<1) > 0
	return nil
}

// RXTimingSetupReqPayload represents the RXTimingSetupReq payload.
type RXTimingSetupReqPayload struct {
	Delay uint8 // 0=1s, 1=1s, 2=2s, ... 15=15s
}

// MarshalBinary marshals the object in binary form.
func (p RXTimingSetupReqPayload) MarshalBinary() ([]byte, error) {
	if p.Delay > 15 {
		return []byte{}, errors.New("lorawan: the max value of Delay is 15")
	}
	return []byte{p.Delay}, nil
}

// UnmarshalBinary decodes the object from binary form.
func (p *RXTimingSetupReqPayload) UnmarshalBinary(data []byte) error {
	if len(data) != 1 {
		return errors.New("lorawan: 1 byte of data is expected")
	}
	p.Delay = data[0]
	return nil
}

// TXParamSetupReqPayload represents the TXParamSetupReq payload.
type TXParamSetupReqPayload struct {
	DownlinkDwelltime DwellTime
	UplinkDwellTime   DwellTime
	MaxEIRP           uint8
}

// MarshalBinary encodes the object into a bytes.
func (p TXParamSetupReqPayload) MarshalBinary() ([]byte, error) {
	var b uint8
	for i, v := range []uint8{8, 10, 12, 13, 14, 16, 18, 20, 21, 24, 26, 27, 29, 30, 33, 36} {
		if v == p.MaxEIRP {
			b = uint8(i)
		}
	}
	if b == 0 {
		return nil, errors.New("lorawan: invalid MaxEIRP value")
	}

	if p.UplinkDwellTime == DwellTime400ms {
		b = b ^ (1 << 4)
	}
	if p.DownlinkDwelltime == DwellTime400ms {
		b = b ^ (1 << 5)
	}

	return []byte{b}, nil
}

// UnmarshalBinary decodes the object from bytes.
func (p *TXParamSetupReqPayload) UnmarshalBinary(data []byte) error {
	if len(data) != 1 {
		return errors.New("lorawan: 1 byte of data is expected")
	}

	if data[0]&(1<<4) > 0 {
		p.UplinkDwellTime = DwellTime400ms
	}
	if data[0]&(1<<5) > 0 {
		p.DownlinkDwelltime = DwellTime400ms
	}
	p.MaxEIRP = []uint8{8, 10, 12, 13, 14, 16, 18, 20, 21, 24, 26, 27, 29, 30, 33, 36}[data[0]&15]

	return nil
}

// DLChannelReqPayload represents the DLChannelReq payload.
type DLChannelReqPayload struct {
	ChIndex uint8
	Freq    uint32
}

// MarshalBinary encodes the object into bytes.
func (p DLChannelReqPayload) MarshalBinary() ([]byte, error) {
	b := make([]byte, 5)        // we need one byte more for PutUint32
	if p.Freq/100 >= 16777216 { // 2^24
		return b, errors.New("lorawan: max value of Freq is 2^24 - 1")
	}

	if p.Freq%100 != 0 {
		return b, errors.New("lorawan: Freq must be a multiple of 100")
	}

	b[0] = p.ChIndex
	binary.LittleEndian.PutUint32(b[1:5], p.Freq/100)

	return b[0:4], nil
}

// UnmarshalBinary decodes the object from bytes.
func (p *DLChannelReqPayload) UnmarshalBinary(data []byte) error {
	if len(data) != 4 {
		return errors.New("lorawan: 4 bytes of data are expected")
	}

	p.ChIndex = data[0]
	b := make([]byte, 4)
	copy(b, data[1:])
	p.Freq = binary.LittleEndian.Uint32(b) * 100
	return nil
}

// DLChannelAnsPayload represents the DLChannelAns payload.
type DLChannelAnsPayload struct {
	UplinkFrequencyExists bool
	ChannelFrequencyOK    bool
}

// MarshalBinary encodes the object into bytes.
func (p DLChannelAnsPayload) MarshalBinary() ([]byte, error) {
	var b byte
	if p.ChannelFrequencyOK {
		b = b ^ 1
	}
	if p.UplinkFrequencyExists {
		b = b ^ (1 << 1)
	}
	return []byte{b}, nil
}

// UnmarshalBinary decodes the object from bytes.
func (p *DLChannelAnsPayload) UnmarshalBinary(data []byte) error {
	if len(data) != 1 {
		return errors.New("lorawan: 1 byte of data is expected")
	}

	p.ChannelFrequencyOK = data[0]&1 > 0
	p.UplinkFrequencyExists = data[0]&(1<<1) > 0
	return nil
}
