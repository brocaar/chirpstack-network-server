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

// ShiftLeft shifts the binary string left by one bit, causing the
// most-signficant bit to disappear and a zero to be introduced at the right.
// This corresponds to the `x << 1` notation of RFC 4493.
func ShiftLeft(b []byte) []byte {
	l := len(b)
	if l == 0 {
		panic("shiftLeft requires a non-empty buffer.")
	}

	output := make([]byte, l)

	overflow := byte(0)
	for i := int(l - 1); i >= 0; i-- {
		output[i] = b[i] << 1
		output[i] |= overflow
		overflow = (b[i] & 0x80) >> 7
	}

	return output
}
