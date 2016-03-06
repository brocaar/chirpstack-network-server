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

import "log"

// Xor computes `a XOR b`, as defined by RFC 4493. dst, a, and b must all have
// the same length.
func Xor(dst []byte, a []byte, b []byte) {
	// TODO(jacobsa): Consider making this a helper function with known sizes
	// where it is most hot, then even trying to inline it entirely.

	if len(dst) != len(a) || len(a) != len(b) {
		log.Panicf("Bad buffer lengths: %d, %d, %d", len(dst), len(a), len(b))
	}

	for i, _ := range a {
		dst[i] = a[i] ^ b[i]
	}
}
