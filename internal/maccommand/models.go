package maccommand

import (
	"github.com/Frankz/lorawan"
	"github.com/pkg/errors"
)

// Block defines a block of MAC commands that must be sent together.
type Block struct {
	CID         lorawan.CID
	FRMPayload  bool // command must be sent as a FRMPayload (and thus encrypted)
	External    bool // command was enqueued by an external service
	MACCommands MACCommands
}

// Size returns the size (in bytes) of the mac-commands within this block.
func (m *Block) Size() (int, error) {
	var count int
	for _, mc := range m.MACCommands {
		b, err := mc.MarshalBinary()
		if err != nil {
			return 0, errors.Wrap(err, "marshal binary error")
		}
		count += len(b)
	}
	return count, nil
}

// MACCommands holds a slice of MACCommand items.
type MACCommands []lorawan.MACCommand

// MarshalBinary implements the encoding.BinaryMarshaler interface.
func (m MACCommands) MarshalBinary() ([]byte, error) {
	var out []byte
	for _, mac := range m {
		b, err := mac.MarshalBinary()
		if err != nil {
			return nil, err
		}
		out = append(out, b...)
	}
	return out, nil
}

// UnmarshalBinary implements the encoding.BinaryUnmarshaler interface.
func (m *MACCommands) UnmarshalBinary(data []byte) error {
	var pLen int
	for i := 0; i < len(data); i++ {
		if _, s, err := lorawan.GetMACPayloadAndSize(false, lorawan.CID(data[i])); err != nil {
			pLen = 0
		} else {
			pLen = s
		}

		// check if the remaining bytes are >= CID byte + payload size
		if len(data[i:]) < pLen+1 {
			return errors.New("not enough remaining bytes")
		}

		var mc lorawan.MACCommand
		if err := mc.UnmarshalBinary(false, data[i:i+1+pLen]); err != nil {
			return err
		}
		*m = append(*m, mc)
		i += pLen
	}
	return nil
}
