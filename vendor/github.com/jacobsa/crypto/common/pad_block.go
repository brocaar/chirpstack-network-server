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

package common

import (
	"crypto/aes"
)

// PadBlock pads a string of bytes less than 16 bytes long to a full block size
// by appending a one bit followed by zero bits. This is the padding function
// used in RFCs 4493 and 5297.
func PadBlock(block []byte) []byte {
	blockLen := len(block)
	if blockLen >= aes.BlockSize {
		panic("PadBlock input must be less than 16 bytes.")
	}

	result := make([]byte, aes.BlockSize)
	copy(result, block)
	result[blockLen] = 0x80

	return result
}
