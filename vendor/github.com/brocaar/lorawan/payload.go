package lorawan

import (
	"encoding"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// EUI64 data type
type EUI64 [8]byte

// MarshalJSON implements json.Marshaler.
func (e EUI64) MarshalJSON() ([]byte, error) {
	return []byte(`"` + e.String() + `"`), nil
}

// UnmarshalJSON implements json.Unmarshaler.
func (e *EUI64) UnmarshalJSON(data []byte) error {
	hexStr := strings.Trim(string(data), `"`)
	b, err := hex.DecodeString(hexStr)
	if err != nil {
		return err
	}
	if len(b) != len(e) {
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
type Payload interface {
	encoding.BinaryMarshaler
	encoding.BinaryUnmarshaler
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
func (p *DataPayload) UnmarshalBinary(data []byte) error {
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
func (p *JoinRequestPayload) UnmarshalBinary(data []byte) error {
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

// JoinAcceptPayload represents the join-accept message payload.
// todo: implement CFlist
type JoinAcceptPayload struct {
	AppNonce   [3]byte
	NetID      [3]byte
	DevAddr    DevAddr
	DLSettings DLsettings
	RXDelay    uint8
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

	return out, nil
}

// UnmarshalBinary decodes the object from binary form.
func (p *JoinAcceptPayload) UnmarshalBinary(data []byte) error {
	if len(data) != 12 {
		return errors.New("lorawan: 12 bytes of data are expected")
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
	return nil
}
