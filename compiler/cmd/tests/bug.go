// Copyright 2026 Richard Kelsey. All rights reserved.
// See file LICENSE for notices and license.

package tests

// Tests for known bugs.  Their entries in compile.go's test table are
// commented out.  When a bug is fixed, its test moves to regression.go
// and its entry is enabled.

// Literals that do not fit in 12 bits.  loadConstant only uses 'li'
// (addi), so this panics with "iFormat imm12 out of range".
// x = 1 -> 30001, 19088743
func bigLiteral(x int) int {
	print(x + 30000)
	print((x + 0x123456789) >> 8)
	return 0
}

// print only prints the low 32 bits, sign-extended (it does
// 'addw a0, a0, zero').
// x = 3000000000 -> 3000000000
func printBig(x int) int {
	print(x)
	return 0
}

// Printing a narrow value directly, without converting it to int
// first.  The register holds 200 in its low byte, but the bits above
// it are not the sign extension.
// x = 200 -> -56
func printNarrow(x int) int {
	a := int8(x)
	print(a)
	return 0
}

// The operations below use all of a register's bits, so a narrow
// value has to be extended before they are applied to it.  In each,
// x = 200 and a + a, 400, carries into bit 8, giving 144 as a uint8.

// x = 200 -> 1, 0
func narrowEq(x int) int {
	a := uint8(x)
	a = a + a
	r0 := 0
	if a == 144 {
		r0 = 1
	}
	r1 := 0
	if a != 144 {
		r1 = 1
	}
	print(r0)
	print(r1)
	return 0
}

// x = 200 -> 1, 1
func narrowLt(x int) int {
	a := uint8(x)
	a = a + a
	r0 := 0
	if a < 150 {
		r0 = 1
	}
	// int8(200) is -56.
	b := int8(x)
	r1 := 0
	if b < 0 {
		r1 = 1
	}
	print(r0)
	print(r1)
	return 0
}

// x = 200 -> 72, 4
func narrowDiv(x int) int {
	a := uint8(x)
	a = a + a
	print(int(a / 2))
	print(int(a % 10))
	return 0
}

// x = 200 -> 72
func narrowShr(x int) int {
	a := uint8(x)
	a = a + a
	print(int(a >> 1))
	return 0
}

// Constant folding through narrow types.  The arithmetic has to wrap
// at the type's size and convertInt has to fold (see the comment on
// convertIntPrimopT.Simplify).  Like the ...Const tests in arith.go,
// this should compile to 7 instructions.
// -> 44
func convConst() int {
	var z uint8 = 200
	z = z + 100
	print(int(z))
	return 0
}
