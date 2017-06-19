package lorawan

import (
	"errors"
	"fmt"
	"log"
)

// MACPayload represents the MAC payload. Use NewMACPayload for creating a new
// MACPayload.
type MACPayload struct {
	FHDR       FHDR      `json:"fhdr"`
	FPort      *uint8    `json:"fPort"` // optional, but must be set when FRMPayload is set
	FRMPayload []Payload `json:"frmPayload"`
}

func (p MACPayload) marshalPayload() ([]byte, error) {
	var out []byte
	var b []byte
	var err error
	for _, fp := range p.FRMPayload {
		if mac, ok := fp.(*MACCommand); ok {
			if p.FPort == nil || (p.FPort != nil && *p.FPort != 0) {
				return []byte{}, errors.New("lorawan: a MAC command is only allowed when FPort=0")
			}
			b, err = mac.MarshalBinary()
		} else {
			b, err = fp.MarshalBinary()
		}
		if err != nil {
			return nil, err
		}
		out = append(out, b...)
	}
	return out, nil
}

func (p *MACPayload) decodeFRMPayloadToMACCommands(uplink bool) error {
	if p.FPort == nil || *p.FPort != 0 {
		return fmt.Errorf("lorawan: FPort must be 0 when calling decodeFRMPayloadToMACCommands")
	}

	if len(p.FRMPayload) != 1 {
		return fmt.Errorf("lorawan: exactly 1 Payload was expected in FRMPayload")
	}

	dataPL, ok := p.FRMPayload[0].(*DataPayload)
	if !ok {
		return fmt.Errorf("lorawan: expected *DataPayload, got %T", p.FRMPayload[0])
	}

	var pLen int
	p.FRMPayload = make([]Payload, 0)
	for i := 0; i < len(dataPL.Bytes); i++ {
		if _, s, err := GetMACPayloadAndSize(uplink, CID(dataPL.Bytes[i])); err != nil {
			pLen = 0
		} else {
			pLen = s
		}

		// check if the remaining bytes are >= CID byte + payload size
		if len(dataPL.Bytes[i:]) < pLen+1 {
			return errors.New("lorawan: not enough remaining bytes")
		}

		mc := &MACCommand{}
		if err := mc.UnmarshalBinary(uplink, dataPL.Bytes[i:i+1+pLen]); err != nil {
			log.Printf("warning: unmarshal mac-command error (skipping remaining mac-command bytes): %s", err)
			break
		}
		p.FRMPayload = append(p.FRMPayload, mc)

		// go to the next command (skip the payload bytes of the current command)
		i = i + pLen
	}

	return nil
}

// MarshalBinary marshals the object in binary form.
func (p MACPayload) MarshalBinary() ([]byte, error) {
	var b []byte
	var out []byte
	var err error

	b, err = p.FHDR.MarshalBinary()
	if err != nil {
		return nil, err
	}
	out = append(out, b...)

	if p.FPort == nil {
		if len(p.FRMPayload) != 0 {
			return nil, errors.New("lorawan: FPort must be set when FRMPayload is not empty")
		}
		return out, nil
	} else if len(p.FHDR.FOpts) != 0 && *p.FPort == 0 {
		return nil, errors.New("lorawan: FPort must not be 0 when FOpts are set")
	}

	out = append(out, *p.FPort)

	if b, err = p.marshalPayload(); err != nil {
		return nil, err
	}
	out = append(out, b...)
	return out, nil
}

// UnmarshalBinary decodes the object from binary form.
func (p *MACPayload) UnmarshalBinary(uplink bool, data []byte) error {
	dataLen := len(data)

	// check that there are enough bytes to decode a minimal FHDR
	if dataLen < 7 {
		return errors.New("lorawan: at least 7 bytes needed to decode FHDR")
	}

	// unmarshal FCtrl so we know the FOptsLen
	if err := p.FHDR.FCtrl.UnmarshalBinary(data[4:5]); err != nil {
		return err
	}

	// check that there are at least as many bytes as FOptsLen claims
	if dataLen < 7+int(p.FHDR.FCtrl.fOptsLen) {
		return errors.New("lorawan: not enough bytes to decode FHDR")
	}

	// decode the full FHDR (including optional FOpts)
	if err := p.FHDR.UnmarshalBinary(uplink, data[0:7+p.FHDR.FCtrl.fOptsLen]); err != nil {
		return err
	}

	// decode the optional FPort
	if dataLen >= 7+int(p.FHDR.FCtrl.fOptsLen)+1 {
		fPort := uint8(data[7+int(p.FHDR.FCtrl.fOptsLen)])
		p.FPort = &fPort
	}

	// decode the rest of the payload (if present)
	if dataLen > 7+int(p.FHDR.FCtrl.fOptsLen)+1 {
		if p.FPort != nil && *p.FPort == 0 && p.FHDR.FCtrl.fOptsLen > 0 {
			return errors.New("lorawan: FPort must not be 0 when FOpts are set")
		}

		// even when FPort = 0, we store the mac-commands within a DataPayload.
		// only after decryption we're able to unmarshal them.
		p.FRMPayload = []Payload{&DataPayload{Bytes: data[7+p.FHDR.FCtrl.fOptsLen+1:]}}
	}

	return nil
}
