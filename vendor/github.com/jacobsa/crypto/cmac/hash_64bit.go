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

// +build amd64 arm64 ppc64 ppc64le s390x mips64 mips64le

// This code assumes that it's safe to perform unaligned word-sized loads. This is safe on:
//  - arm64 per http://infocenter.arm.com/help/index.jsp?topic=/com.arm.doc.den0024a/ch05s01s02.html
//  - Section "5.5.8 Alignment Interrupt" of PowerPC Operating Environment Architecture Book III Version 2.02 
//    (the first PowerPC ISA version to include 64-bit), available from 
//    http://www.ibm.com/developerworks/systems/library/es-archguide-v2.html does not permit fixed-point loads
//    or stores to generate exceptions on unaligned access
//  - IBM mainframe's have allowed unaligned accesses since the System/370 arrived in 1970
//  - On mips unaligned accesses are fixed up by the kernel per https://www.linux-mips.org/wiki/Alignment
//    so performance might be quite bad but it will work.

package cmac

import (
	"log"
	"unsafe"
)

// XOR the blockSize bytes starting at a and b, writing the result over dst.
func xorBlock(
	dstPtr unsafe.Pointer,
	aPtr unsafe.Pointer,
	bPtr unsafe.Pointer) {
	// Check assumptions. (These are compile-time constants, so this should
	// compile out.)
	const wordSize = unsafe.Sizeof(uintptr(0))
	if blockSize != 2*wordSize {
		log.Panicf("%d %d", blockSize, wordSize)
	}

	// Convert.
	a := (*[2]uintptr)(aPtr)
	b := (*[2]uintptr)(bPtr)
	dst := (*[2]uintptr)(dstPtr)

	// Compute.
	dst[0] = a[0] ^ b[0]
	dst[1] = a[1] ^ b[1]
}
