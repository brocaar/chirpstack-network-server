package rejoin

import (
	"crypto/aes"
	"fmt"

	keywrap "github.com/NickBall/go-aes-key-wrap"
	"github.com/pkg/errors"

	"github.com/brocaar/lorawan"
	"github.com/brocaar/lorawan/backend"
)

// unwrapNSKeyEnveope returns the decrypted key from the given KeyEnvelope.
func unwrapNSKeyEnvelope(ke *backend.KeyEnvelope) (lorawan.AES128Key, error) {
	var key lorawan.AES128Key

	if ke.KEKLabel == "" {
		copy(key[:], ke.AESKey[:])
		return key, nil
	}

	kek, ok := keks[ke.KEKLabel]
	if !ok {
		return key, fmt.Errorf("unknown kek label: %s", ke.KEKLabel)
	}

	block, err := aes.NewCipher(kek)
	if err != nil {
		return key, errors.Wrap(err, "new cipher error")
	}

	b, err := keywrap.Unwrap(block, ke.AESKey[:])
	if err != nil {
		return key, errors.Wrap(err, "unwrap key error")
	}

	copy(key[:], b)
	return key, nil
}
