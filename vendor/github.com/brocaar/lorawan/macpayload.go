package lorawan

import (
	"crypto/aes"
	"encoding/binary"
	"errors"
)

// MACPayload represents the MAC payload. Use NewMACPayload for creating a new
// MACPayload.
type MACPayload struct {
	FHDR       FHDR
	FPort      uint8 // ignored when FRMPayload is empty
	FRMPayload []Payload
	uplink     bool // used for binary (un)marshaling and encryption / decryption
}

// NewMACPayload returns a new MACPayload set to either uplink or downlink.
// This is needed since there is a difference in how uplink and downlink
// payloads are (un)marshalled and encrypted / decrypted.
func NewMACPayload(uplink bool) *MACPayload {
	return &MACPayload{
		uplink: uplink,
	}
}

func (p MACPayload) marshalPayload() ([]byte, error) {
	var out []byte
	var b []byte
	var err error
	for _, fp := range p.FRMPayload {
		if mac, ok := fp.(*MACCommand); ok {
			if p.FPort != 0 {
				return []byte{}, errors.New("lorawan: a MAC command is only allowed when FPort=0")
			}
			mac.uplink = p.uplink
			b, err = mac.MarshalBinary()
		} else {
			b, err = fp.MarshalBinary()
		}
		if err != nil {
			return []byte{}, err
		}
		out = append(out, b...)
	}
	return out, nil
}

func (p *MACPayload) unmarshalPayload(data []byte) error {
	if p.FPort == 0 {
		// payload contains MAC commands
		var pLen int
		p.FRMPayload = make([]Payload, 0)
		for i := 0; i < len(data); i++ {
			if _, s, err := getMACPayloadAndSize(p.uplink, cid(data[i])); err != nil {
				pLen = 0
			} else {
				pLen = s
			}

			// check if the remaining bytes are >= CID byte + payload size
			if len(data[i:]) < pLen+1 {
				return errors.New("lorawan: not enough remaining bytes")
			}

			mc := &MACCommand{uplink: p.uplink}
			if err := mc.UnmarshalBinary(data[i : i+1+pLen]); err != nil {
				return err
			}
			p.FRMPayload = append(p.FRMPayload, mc)

			// go to the next command (skip the payload bytes of the current command)
			i = i + pLen
		}

	} else {
		// payload contains user defined data
		p.FRMPayload = []Payload{&DataPayload{}}
		if err := p.FRMPayload[0].UnmarshalBinary(data); err != nil {
			return err
		}
	}
	return nil
}

// MarshalBinary marshals the object in binary form.
func (p MACPayload) MarshalBinary() ([]byte, error) {
	var b []byte
	var out []byte
	var err error

	p.FHDR.uplink = p.uplink
	b, err = p.FHDR.MarshalBinary()
	if err != nil {
		return []byte{}, err
	}
	out = append(out, b...)

	// ignore FPort and FRMPayload when FRMPayload is empty
	if len(p.FRMPayload) == 0 {
		return out, nil
	}

	out = append(out, p.FPort)

	if b, err = p.marshalPayload(); err != nil {
		return []byte{}, err
	}
	out = append(out, b...)

	return out, nil
}

// EncryptFRMPayload encrypts the FRMPayload with the given key.
func (p *MACPayload) EncryptFRMPayload(key AES128Key) error {
	if len(p.FRMPayload) == 0 {
		return errors.New("lorawan: nothing to encrypt")
	}

	data, err := p.marshalPayload()
	if err != nil {
		return err
	}
	pLen := len(data)
	if pLen%16 != 0 {
		// append with empty bytes so that len(data) is a multiple of 16
		data = append(data, make([]byte, 16-(pLen%16))...)
	}

	block, err := aes.NewCipher(key[:])
	if err != nil {
		return err
	}
	if block.BlockSize() != 16 {
		return errors.New("lorawan: block size of 16 was expected")
	}

	s := make([]byte, 16)
	a := make([]byte, 16)
	a[0] = 0x01
	if !p.uplink {
		a[5] = 0x01
	}

	b, err := p.FHDR.DevAddr.MarshalBinary()
	if err != nil {
		return err
	}
	copy(a[6:10], b)
	binary.LittleEndian.PutUint32(a[10:14], uint32(p.FHDR.FCnt))

	for i := 0; i < len(data)/16; i++ {
		a[15] = byte(i + 1)
		block.Encrypt(s, a)

		for j := 0; j < len(s); j++ {
			data[i*16+j] = data[i*16+j] ^ s[j]
		}
	}

	// store the encrypted data in a DataPayload
	p.FRMPayload = []Payload{&DataPayload{Bytes: data[0:pLen]}}

	return nil
}

// DecryptFRMPayload decrypts the FRMPayload with the given key.
func (p *MACPayload) DecryptFRMPayload(key AES128Key) error {
	if err := p.EncryptFRMPayload(key); err != nil {
		return err
	}
	if len(p.FRMPayload) != 1 {
		return errors.New("lorawan: a single FRMPayload was expected after decrypting")
	}
	dp, ok := p.FRMPayload[0].(*DataPayload)
	if !ok {
		return errors.New("lorawan: a DataPayload was expected")
	}

	return p.unmarshalPayload(dp.Bytes)
}

// UnmarshalBinary decodes the object from binary form.
func (p *MACPayload) UnmarshalBinary(data []byte) error {
	// check that there are enough bytes to decode a minimal FHDR
	if len(data) < 7 {
		return errors.New("lorawan: at least 7 bytes needed to decode FHDR")
	}

	// unmarshal FCtrl so we know the FOptsLen
	if err := p.FHDR.FCtrl.UnmarshalBinary(data[4:5]); err != nil {
		return err
	}

	// check that there are at least as many bytes as FOptsLen claims
	if len(data) < 7+int(p.FHDR.FCtrl.fOptsLen) {
		return errors.New("lorawan: not enough bytes to decode FHDR")
	}

	// decode the full FHDR (including optional FOpts)
	if err := p.FHDR.UnmarshalBinary(data[0 : 7+p.FHDR.FCtrl.fOptsLen]); err != nil {
		return err
	}

	// check that there are at least 2 more bytes (FPort and FRMPayload)
	if len(data) < 7+int(p.FHDR.FCtrl.fOptsLen)+2 {
		if len(data) == 7+int(p.FHDR.FCtrl.fOptsLen)+1 {
			return errors.New("lorawan: data contains FPort but no FRMPayload")
		}
		return nil
	}

	p.FPort = uint8(data[7+p.FHDR.FCtrl.fOptsLen])
	if err := p.unmarshalPayload(data[7+p.FHDR.FCtrl.fOptsLen+1:]); err != nil {
		return err
	}

	return nil
}
