package lorawan

import (
	"encoding/hex"
	"fmt"
)

// NetID represents the NetID.
type NetID [3]byte

// String implements fmt.Stringer.
func (n NetID) String() string {
	return hex.EncodeToString(n[:])
}

// MarshalText implements encoding.TextMarshaler.
func (n NetID) MarshalText() ([]byte, error) {
	return []byte(n.String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (n *NetID) UnmarshalText(text []byte) error {
	b, err := hex.DecodeString(string(text))
	if err != nil {
		return err
	}
	if len(b) != len(n) {
		return fmt.Errorf("lorawan: exactly %d bytes are expected", len(n))
	}
	copy(n[:], b)
	return nil
}

// NwkID returns the NwkID bits of the NetID.
func (n NetID) NwkID() byte {
	return n[2] & 127 // 7 lsb
}
