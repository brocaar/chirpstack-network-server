package lorawan

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
)

// EUI64 data type
type EUI64 [8]byte

// MarshalText implements encoding.TextMarshaler.
func (e EUI64) MarshalText() ([]byte, error) {
	return []byte(e.String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (e *EUI64) UnmarshalText(text []byte) error {
	b, err := hex.DecodeString(string(text))
	if err != nil {
		return err
	}
	if len(e) != len(b) {
		return fmt.Errorf("lorawan: exactly %d bytes are expected", len(e))
	}
	copy(e[:], b)
	return nil
}

// String implement fmt.Stringer.
func (e EUI64) String() string {
	return hex.EncodeToString(e[:])
}

// MarshalBinary implements encoding.BinaryMarshaler.
func (e EUI64) MarshalBinary() ([]byte, error) {
	out := make([]byte, len(e))
	// little endian
	for i, v := range e {
		out[len(e)-i-1] = v
	}
	return out, nil
}

// UnmarshalBinary implements encoding.BinaryUnmarshaler.
func (e *EUI64) UnmarshalBinary(data []byte) error {
	if len(data) != len(e) {
		return fmt.Errorf("lorawan: %d bytes of data are expected", len(e))
	}
	for i, v := range data {
		// little endian
		e[len(e)-i-1] = v
	}
	return nil
}

// Scan implements sql.Scanner.
func (a *EUI64) Scan(src interface{}) error {
	b, ok := src.([]byte)
	if !ok {
		return errors.New("lorawan: []byte type expected")
	}
	if len(b) != len(a) {
		return fmt.Errorf("lorawan []byte must have length %d", len(a))
	}
	copy(a[:], b)
	return nil
}

// Payload is the interface that every payload needs to implement.
// Since it might be a MACPayload, an indication must be given if
// the direction is uplink or downlink (it has different payloads
// for the same CID, based on direction).
type Payload interface {
	MarshalBinary() (data []byte, err error)
	UnmarshalBinary(uplink bool, data []byte) error
}

// DataPayload represents a slice of bytes.
type DataPayload struct {
	Bytes []byte
}

// MarshalBinary marshals the object in binary form.
func (p DataPayload) MarshalBinary() ([]byte, error) {
	return p.Bytes, nil
}

// UnmarshalBinary decodes the object from binary form.
func (p *DataPayload) UnmarshalBinary(uplink bool, data []byte) error {
	p.Bytes = make([]byte, len(data))
	copy(p.Bytes, data)
	return nil
}

// JoinRequestPayload represents the join-request message payload.
type JoinRequestPayload struct {
	AppEUI   EUI64
	DevEUI   EUI64
	DevNonce [2]byte
}

// MarshalBinary marshals the object in binary form.
func (p JoinRequestPayload) MarshalBinary() ([]byte, error) {
	out := make([]byte, 0, 18)
	b, err := p.AppEUI.MarshalBinary()
	if err != nil {
		return nil, err
	}
	out = append(out, b...)
	b, err = p.DevEUI.MarshalBinary()
	if err != nil {
		return nil, err
	}
	out = append(out, b...)
	// little endian
	out = append(out, p.DevNonce[1], p.DevNonce[0])
	return out, nil
}

// UnmarshalBinary decodes the object from binary form.
func (p *JoinRequestPayload) UnmarshalBinary(uplink bool, data []byte) error {
	if len(data) != 18 {
		return errors.New("lorawan: 18 bytes of data are expected")
	}
	if err := p.AppEUI.UnmarshalBinary(data[0:8]); err != nil {
		return err
	}
	if err := p.DevEUI.UnmarshalBinary(data[8:16]); err != nil {
		return err
	}
	// little endian
	p.DevNonce[1] = data[16]
	p.DevNonce[0] = data[17]
	return nil
}

// CFList represents a list of channel frequencies. Each frequency is in Hz
// and must be multiple of 100, (since the frequency will be divided by 100
// on encoding), the max allowed value is 2^24-1 * 100.
type CFList [5]uint32

// MarshalBinary marshals the object in binary form.
func (l CFList) MarshalBinary() ([]byte, error) {
	out := make([]byte, 0, 16)
	for _, f := range l {
		if f%100 != 0 {
			return nil, errors.New("lorawan: frequency must be a multiple of 100")
		}
		f = f / 100
		if f > 16777215 { // 2^24 - 1
			return nil, errors.New("lorawan: max value of frequency is 2^24-1")
		}
		b := make([]byte, 4, 4)
		binary.LittleEndian.PutUint32(b, f)
		out = append(out, b[:3]...)
	}
	// last byte is 0 / RFU
	return append(out, 0), nil
}

// UnmarshalBinary decodes the object from binary form.
func (l *CFList) UnmarshalBinary(data []byte) error {
	if len(data) != 16 {
		return errors.New("lorawan: 16 bytes of data are expected")
	}
	for i := 0; i < 5; i++ {
		l[i] = binary.LittleEndian.Uint32([]byte{
			data[i*3],
			data[i*3+1],
			data[i*3+2],
			0,
		}) * 100
	}

	return nil
}

// JoinAcceptPayload represents the join-accept message payload.
type JoinAcceptPayload struct {
	AppNonce   [3]byte
	NetID      [3]byte
	DevAddr    DevAddr
	DLSettings DLsettings
	RXDelay    uint8
	CFList     *CFList
}

// MarshalBinary marshals the object in binary form.
func (p JoinAcceptPayload) MarshalBinary() ([]byte, error) {
	out := make([]byte, 0, 12)

	// little endian
	for i := len(p.AppNonce) - 1; i >= 0; i-- {
		out = append(out, p.AppNonce[i])
	}
	for i := len(p.NetID) - 1; i >= 0; i-- {
		out = append(out, p.NetID[i])
	}

	b, err := p.DevAddr.MarshalBinary()
	if err != nil {
		return nil, err
	}
	out = append(out, b...)

	b, err = p.DLSettings.MarshalBinary()
	if err != nil {
		return nil, err
	}
	out = append(out, b...)
	out = append(out, byte(p.RXDelay))

	if p.CFList != nil {
		b, err = p.CFList.MarshalBinary()
		if err != nil {
			return nil, err
		}
		out = append(out, b...)
	}

	return out, nil
}

// UnmarshalBinary decodes the object from binary form.
func (p *JoinAcceptPayload) UnmarshalBinary(uplink bool, data []byte) error {
	l := len(data)
	if l != 12 && l != 28 {
		return errors.New("lorawan: 12 or 28 bytes of data are expected (28 bytes if CFList is present)")
	}

	// little endian
	for i, v := range data[0:3] {
		p.AppNonce[2-i] = v
	}
	for i, v := range data[3:6] {
		p.NetID[2-i] = v
	}

	if err := p.DevAddr.UnmarshalBinary(data[6:10]); err != nil {
		return err
	}
	if err := p.DLSettings.UnmarshalBinary(data[10:11]); err != nil {
		return err
	}
	p.RXDelay = uint8(data[11])

	if l == 28 {
		p.CFList = &CFList{}
		if err := p.CFList.UnmarshalBinary(data[12:]); err != nil {
			return err
		}
	}

	return nil
}
