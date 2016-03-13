//go:generate stringer -type=MType

package lorawan

import (
	"crypto/aes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/jacobsa/crypto/cmac"
)

// MType represents the message type.
type MType byte

// Major defines the major version of data message.
type Major byte

// Supported message types (MType)
const (
	JoinRequest MType = iota
	JoinAccept
	UnconfirmedDataUp
	UnconfirmedDataDown
	ConfirmedDataUp
	ConfirmedDataDown
	RFU
	Proprietary
)

// Supported major versions
const (
	LoRaWANR1 Major = 0
)

// AES128Key represents a 128 bit AES key.
type AES128Key [16]byte

// String implements fmt.Stringer.
func (k AES128Key) String() string {
	return hex.EncodeToString(k[:])
}

// MarshalText implements encoding.TextMarshaler.
func (k AES128Key) MarshalText() ([]byte, error) {
	return []byte(k.String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (k *AES128Key) UnmarshalText(text []byte) error {
	b, err := hex.DecodeString(string(text))
	if err != nil {
		return err
	}
	if len(b) != len(k) {
		return fmt.Errorf("lorawan: exactly %d bytes are expected", len(k))
	}
	copy(k[:], b)
	return nil
}

// Scan implements sql.Scanner.
func (a *AES128Key) Scan(src interface{}) error {
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

// MHDR represents the MAC header.
type MHDR struct {
	MType MType
	Major Major
}

// MarshalBinary marshals the object in binary form.
func (h MHDR) MarshalBinary() ([]byte, error) {
	return []byte{byte(h.Major) ^ (byte(h.MType) << 5)}, nil
}

// UnmarshalBinary decodes the object from binary form.
func (h *MHDR) UnmarshalBinary(data []byte) error {
	if len(data) != 1 {
		return errors.New("lorawan: 1 byte of data is expected")
	}
	h.Major = Major(data[0] & 3)
	h.MType = MType((data[0] & 224) >> 5)
	return nil
}

// PHYPayload represents the physical payload. Use NewPhyPayload for creating
// a new PHYPayload.
type PHYPayload struct {
	MHDR       MHDR
	MACPayload Payload
	MIC        [4]byte
	uplink     bool
}

// NewPHYPayload returns a new PHYPayload instance set to either uplink or downlink.
// This is needed since there is a difference in how uplink and downlink
// payloads are (un)marshalled and encrypted / decrypted.
func NewPHYPayload(uplink bool) PHYPayload {
	return PHYPayload{uplink: uplink}
}

// calculateMIC calculates and returns the MIC.
func (p PHYPayload) calculateMIC(key AES128Key) ([]byte, error) {
	if p.MACPayload == nil {
		return []byte{}, errors.New("lorawan: MACPayload should not be empty")
	}

	macPayload, ok := p.MACPayload.(*MACPayload)
	if !ok {
		return []byte{}, errors.New("lorawan: MACPayload should be of type *MACPayload")
	}

	var b []byte
	var err error
	var micBytes []byte

	b, err = p.MHDR.MarshalBinary()
	if err != nil {
		return nil, err
	}
	micBytes = append(micBytes, b...)

	b, err = macPayload.MarshalBinary()
	if err != nil {
		return nil, err
	}
	micBytes = append(micBytes, b...)

	b0 := make([]byte, 16)
	b0[0] = 0x49
	if !p.uplink {
		b0[5] = 1
	}
	b, err = macPayload.FHDR.DevAddr.MarshalBinary()
	if err != nil {
		return nil, err
	}
	copy(b0[6:10], b)
	binary.LittleEndian.PutUint32(b0[10:14], macPayload.FHDR.FCnt)
	b0[15] = byte(len(micBytes))

	hash, err := cmac.New(key[:])
	if err != nil {
		return nil, err
	}

	if _, err = hash.Write(b0); err != nil {
		return nil, err
	}
	if _, err = hash.Write(micBytes); err != nil {
		return nil, err
	}

	hb := hash.Sum([]byte{})
	if len(hb) < 4 {
		return nil, errors.New("lorawan: the hash returned less than 4 bytes")
	}
	return hb[0:4], nil
}

// calculateJoinRequestMIC calculates and returns the join-request MIC.
func (p PHYPayload) calculateJoinRequestMIC(key AES128Key) ([]byte, error) {
	if p.MACPayload == nil {
		return []byte{}, errors.New("lorawan: MACPayload should not be empty")
	}
	jrPayload, ok := p.MACPayload.(*JoinRequestPayload)
	if !ok {
		return []byte{}, errors.New("lorawan: MACPayload should be of type *JoinRequestPayload")
	}

	micBytes := make([]byte, 0, 19)

	b, err := p.MHDR.MarshalBinary()
	if err != nil {
		return []byte{}, err
	}
	micBytes = append(micBytes, b...)

	b, err = jrPayload.MarshalBinary()
	if err != nil {
		return nil, err
	}
	micBytes = append(micBytes, b...)

	hash, err := cmac.New(key[:])
	if err != nil {
		return []byte{}, err
	}
	if _, err = hash.Write(micBytes); err != nil {
		return nil, err
	}
	hb := hash.Sum([]byte{})
	if len(hb) < 4 {
		return []byte{}, errors.New("lorawan: the hash returned less than 4 bytes")
	}
	return hb[0:4], nil
}

// calculateJoinAcceptMIC calculates and returns the join-accept MIC.
func (p PHYPayload) calculateJoinAcceptMIC(key AES128Key) ([]byte, error) {
	if p.MACPayload == nil {
		return []byte{}, errors.New("lorawan: MACPayload should not be empty")
	}
	jaPayload, ok := p.MACPayload.(*JoinAcceptPayload)
	if !ok {
		return []byte{}, errors.New("lorawan: MACPayload should be of type *JoinAcceptPayload")
	}

	micBytes := make([]byte, 0, 13)

	b, err := p.MHDR.MarshalBinary()
	if err != nil {
		return []byte{}, err
	}
	micBytes = append(micBytes, b...)

	b, err = jaPayload.MarshalBinary()
	if err != nil {
		return nil, err
	}
	micBytes = append(micBytes, b...)

	hash, err := cmac.New(key[:])
	if err != nil {
		return []byte{}, err
	}
	if _, err = hash.Write(micBytes); err != nil {
		return nil, err
	}
	hb := hash.Sum([]byte{})
	if len(hb) < 4 {
		return []byte{}, errors.New("lorawan: the hash returned less than 4 bytes")
	}
	return hb[0:4], nil
}

// SetMIC calculates and sets the MIC field.
func (p *PHYPayload) SetMIC(key AES128Key) error {
	var mic []byte
	var err error

	switch p.MACPayload.(type) {
	case *JoinRequestPayload:
		mic, err = p.calculateJoinRequestMIC(key)
	case *JoinAcceptPayload:
		mic, err = p.calculateJoinAcceptMIC(key)
	default:
		mic, err = p.calculateMIC(key)
	}

	if err != nil {
		return err
	}
	if len(mic) != 4 {
		return errors.New("lorawan: a MIC of 4 bytes is expected")
	}
	for i, v := range mic {
		p.MIC[i] = v
	}
	return nil
}

// ValidateMIC returns if the MIC is valid.
// When using 32 bit frame counters, only the least-signification 16 bits are
// sent / received. In order to validate the MIC, the receiver needs to set
// the FCnt to the full 32 bit value (based on the observation of the traffic).
// See section '4.3.1.5 Frame counter (FCnt)' of the LoRaWAN 1.0 specification
// for more details.
func (p PHYPayload) ValidateMIC(key AES128Key) (bool, error) {
	var mic []byte
	var err error

	switch p.MACPayload.(type) {
	case *JoinRequestPayload:
		mic, err = p.calculateJoinRequestMIC(key)
	case *JoinAcceptPayload:
		mic, err = p.calculateJoinAcceptMIC(key)
	default:
		mic, err = p.calculateMIC(key)
	}

	if err != nil {
		return false, err
	}
	if len(mic) != 4 {
		return false, errors.New("lorawan: a MIC of 4 bytes is expected")
	}
	for i, v := range mic {
		if p.MIC[i] != v {
			return false, nil
		}
	}
	return true, nil
}

// EncryptJoinAcceptPayload encrypts the join-accept payload with the given
// AppKey. Note that encrypted must be performed after calling SetMIC
// (sicne the MIC is part of the encrypted payload).
func (p *PHYPayload) EncryptJoinAcceptPayload(appKey AES128Key) error {
	if _, ok := p.MACPayload.(*JoinAcceptPayload); !ok {
		return errors.New("lorawan: MACPayload value must be of type *JoinAcceptPayload")
	}

	pt, err := p.MACPayload.MarshalBinary()
	if err != nil {
		return err
	}

	// in the 1.0 spec instead of DLSettings there is RFU field. the assumption
	// is made that this should have been DLSettings.

	pt = append(pt, p.MIC[0:4]...)
	if len(pt)%16 != 0 {
		return errors.New("lorawan: plaintext must be a multiple of 16 bytes")
	}

	block, err := aes.NewCipher(appKey[:])
	if err != nil {
		return err
	}
	if block.BlockSize() != 16 {
		return errors.New("lorawan: block-size of 16 bytes is expected")
	}
	ct := make([]byte, len(pt))
	for i := 0; i < len(ct)/16; i++ {
		offset := i * 16
		block.Decrypt(ct[offset:offset+16], pt[offset:offset+16])
	}
	p.MACPayload = &DataPayload{Bytes: ct[0 : len(ct)-4]}
	copy(p.MIC[:], ct[len(ct)-4:])
	return nil
}

// DecryptJoinAcceptPayload decrypts the join-accept payload with the given
// AppKey. Note that you need to decrypte before you can validate the MIC.
func (p *PHYPayload) DecryptJoinAcceptPayload(appKey AES128Key) error {
	dp, ok := p.MACPayload.(*DataPayload)
	if !ok {
		return errors.New("lorawan: MACPayload must be of type *DataPayload")
	}

	// append MIC to the ciphertext since it is encrypted too
	ct := append(dp.Bytes, p.MIC[:]...)

	if len(ct)%16 != 0 {
		return errors.New("lorawan: plaintext must be a multiple of 16 bytes")
	}

	block, err := aes.NewCipher(appKey[:])
	if err != nil {
		return err
	}
	if block.BlockSize() != 16 {
		return errors.New("lorawan: block-size of 16 bytes is expected")
	}
	pt := make([]byte, len(ct))
	for i := 0; i < len(pt)/16; i++ {
		offset := i * 16
		block.Encrypt(pt[offset:offset+16], ct[offset:offset+16])
	}

	p.MACPayload = &JoinAcceptPayload{}
	copy(p.MIC[:], pt[len(pt)-4:len(pt)]) // set the decrypted MIC
	return p.MACPayload.UnmarshalBinary(pt[0 : len(pt)-4])
}

// MarshalBinary marshals the object in binary form.
func (p PHYPayload) MarshalBinary() ([]byte, error) {
	if p.MACPayload == nil {
		return []byte{}, errors.New("lorawan: MACPayload should not be nil")
	}

	if mpl, ok := p.MACPayload.(*MACPayload); ok {
		mpl.uplink = p.uplink
	}

	var out []byte
	var b []byte
	var err error

	if b, err = p.MHDR.MarshalBinary(); err != nil {
		return []byte{}, err
	}
	out = append(out, b...)

	if b, err = p.MACPayload.MarshalBinary(); err != nil {
		return []byte{}, err
	}
	out = append(out, b...)
	out = append(out, p.MIC[0:len(p.MIC)]...)
	return out, nil
}

// UnmarshalBinary decodes the object from binary form.
func (p *PHYPayload) UnmarshalBinary(data []byte) error {
	if len(data) < 5 {
		return errors.New("lorawan: at least 5 bytes needed to decode PHYPayload")
	}

	if err := p.MHDR.UnmarshalBinary(data[0:1]); err != nil {
		return err
	}

	switch p.MHDR.MType {
	case JoinRequest:
		p.MACPayload = &JoinRequestPayload{}
	case JoinAccept:
		p.MACPayload = &DataPayload{}
	default:
		p.MACPayload = &MACPayload{uplink: p.uplink}
	}

	if err := p.MACPayload.UnmarshalBinary(data[1 : len(data)-4]); err != nil {
		return err
	}
	for i := 0; i < 4; i++ {
		p.MIC[i] = data[len(data)-4+i]
	}
	return nil
}

// GobEncode implements the gob.GobEncoder interface.
func (p PHYPayload) GobEncode() ([]byte, error) {
	out := make([]byte, 1)
	if p.uplink {
		out[0] = 1
	}

	b, err := p.MarshalBinary()
	if err != nil {
		return nil, err
	}
	out = append(out, b...)
	return out, nil
}

// GobDecode implements the gob.GobEncoder interface.
func (p *PHYPayload) GobDecode(data []byte) error {
	if len(data) < 1 {
		return errors.New("lorawan: at least 1 byte needed for GobDecode")
	}
	if data[0] == 1 {
		p.uplink = true
	}
	return p.UnmarshalBinary(data[1:])
}
