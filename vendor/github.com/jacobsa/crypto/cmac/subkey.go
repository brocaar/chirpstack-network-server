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
	"bytes"
	"crypto/cipher"

	"github.com/jacobsa/crypto/common"
)

var subkeyZero []byte
var subkeyRb []byte

func init() {
	subkeyZero = bytes.Repeat([]byte{0x00}, blockSize)
	subkeyRb = append(bytes.Repeat([]byte{0x00}, blockSize-1), 0x87)
}

// Given the supplied cipher, whose block size must be 16 bytes, return two
// subkeys that can be used in MAC generation. See section 5.3 of NIST SP
// 800-38B. Note that the other NIST-approved block size of 8 bytes is not
// supported by this function.
func generateSubkeys(ciph cipher.Block) (k1 []byte, k2 []byte) {
	if ciph.BlockSize() != blockSize {
		panic("generateSubkeys requires a cipher with a block size of 16 bytes.")
	}

	// Step 1
	l := make([]byte, blockSize)
	ciph.Encrypt(l, subkeyZero)

	// Step 2: Derive the first subkey.
	if common.Msb(l) == 0 {
		// TODO(jacobsa): Accept a destination buffer in ShiftLeft and then hoist
		// the allocation in the else branch below.
		k1 = common.ShiftLeft(l)
	} else {
		k1 = make([]byte, blockSize)
		common.Xor(k1, common.ShiftLeft(l), subkeyRb)
	}

	// Step 3: Derive the second subkey.
	if common.Msb(k1) == 0 {
		k2 = common.ShiftLeft(k1)
	} else {
		k2 = make([]byte, blockSize)
		common.Xor(k2, common.ShiftLeft(k1), subkeyRb)
	}

	return
}
