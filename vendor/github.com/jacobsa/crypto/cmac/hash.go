// Copyright 2012 Aaron Jacobs. All Rights Reserved.
// Author: aaronjjacobs@gmail.com (Aaron Jacobs)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmac

import (
	"crypto/aes"
	"crypto/cipher"
	"fmt"
	"hash"
	"unsafe"

	"github.com/jacobsa/crypto/common"
)

type cmacHash struct {
	// An AES cipher configured with the original key.
	ciph cipher.Block

	// Generated sub-keys.
	k1 []byte
	k2 []byte

	// Data that has been seen by Write but not yet incorporated into x, due to
	// us not being sure if it is the final block or not.
	//
	// INVARIANT: len(data) <= blockSize
	data []byte

	// The current value of X, as defined in the AES-CMAC algorithm in RFC 4493.
	// Initially this is a 128-bit zero, and it is updated with the current block
	// when we're sure it's not the last one.
	x []byte
}

func (h *cmacHash) Write(p []byte) (n int, err error) {
	n = len(p)

	// First step: consume enough data to expand h.data to a full block, if
	// possible.
	{
		toConsume := blockSize - len(h.data)
		if toConsume > len(p) {
			toConsume = len(p)
		}

		h.data = append(h.data, p[:toConsume]...)
		p = p[toConsume:]
	}

	// If there's no data left in p, it means h.data might not be a full block.
	// Even if it is, we're not sure it's the final block, which we must treat
	// specially. So we must stop here.
	if len(p) == 0 {
		return
	}

	// h.data is a full block and is not the last; process it.
	h.writeBlocks(h.data)
	h.data = h.data[:0]

	// Consume any further full blocks in p that we're sure aren't the last. Note
	// that we're sure that len(p) is greater than zero here.
	blocksToProcess := (len(p) - 1) / blockSize
	bytesToProcess := blocksToProcess * blockSize

	h.writeBlocks(p[:bytesToProcess])
	p = p[bytesToProcess:]

	// Store the rest for later.
	h.data = append(h.data, p...)

	return
}

// Process block-aligned data that we're sure does not contain the final block.
//
// REQUIRES: len(p) % blockSize == 0
func (h *cmacHash) writeBlocks(p []byte) {
	y := make([]byte, blockSize)

	for off := 0; off < len(p); off += blockSize {
		block := p[off : off+blockSize]

		xorBlock(
			unsafe.Pointer(&y[0]),
			unsafe.Pointer(&h.x[0]),
			unsafe.Pointer(&block[0]))

		h.ciph.Encrypt(h.x, y)
	}

	return
}

func (h *cmacHash) Sum(b []byte) []byte {
	dataLen := len(h.data)

	// We should have at most one block left.
	if dataLen > blockSize {
		panic(fmt.Sprintf("Unexpected data: %x", h.data))
	}

	// Calculate M_last.
	mLast := make([]byte, blockSize)
	if dataLen == blockSize {
		common.Xor(mLast, h.data, h.k1)
	} else {
		// TODO(jacobsa): Accept a destination buffer in common.PadBlock and
		// simplify this code.
		common.Xor(mLast, common.PadBlock(h.data), h.k2)
	}

	y := make([]byte, blockSize)
	common.Xor(y, mLast, h.x)

	result := make([]byte, blockSize)
	h.ciph.Encrypt(result, y)

	b = append(b, result...)
	return b
}

func (h *cmacHash) Reset() {
	h.data = h.data[:0]
	h.x = make([]byte, blockSize)
}

func (h *cmacHash) Size() int {
	return h.ciph.BlockSize()
}

func (h *cmacHash) BlockSize() int {
	return h.ciph.BlockSize()
}

// New returns an AES-CMAC hash using the supplied key. The key must be 16, 24,
// or 32 bytes long.
func New(key []byte) (hash.Hash, error) {
	switch len(key) {
	case 16, 24, 32:
	default:
		return nil, fmt.Errorf("AES-CMAC requires a 16-, 24-, or 32-byte key.")
	}

	// Create a cipher.
	ciph, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("aes.NewCipher: %v", err)
	}

	// Set up the hash object.
	h := &cmacHash{ciph: ciph}
	h.k1, h.k2 = generateSubkeys(ciph)
	h.Reset()

	return h, nil
}
