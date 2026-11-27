// Copyright 2026 Richard Kelsey. All rights reserved.
// See file LICENSE for notices and license.

package tests

// Regression tests for bugs that have been fixed.  Each test is moved
// here from bug.go when its bug is fixed.

// Unary minus on a variable.  The front end made a '-' call with one
// input, and SimplifyArithBinop indexes a second input.
// x = 3 -> 7
func negVar(x int) int {
	y := -x
	print(y + 10)
	return 0
}

// Unary minus on a constant, as in 'var e int8 = -56'.
// x = 100 -> 44
func negConst(x int) int {
	var e int8 = -56
	print(int(e + int8(x)))
	return 0
}

// Printing a narrow value directly, without converting it to int
// first.  The register held 200 in its low byte, but the bits above
// it were not the sign extension.
// x = 200 -> -56
func printNarrow(x int) int {
	a := int8(x)
	print(a)
	return 0
}

// The operations below use all of a register's bits, so a narrow
// value has to be extended before they are applied to it.  They
// failed when narrow values were not kept extended.  In each, x = 200
// and a + a, 400, carries into bit 8, giving 144 as a uint8.

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

// Literals that do not fit in 12 bits.  loadConstant only used 'li'
// (addi), so this panicked with "iFormat imm12 out of range".
// x = 1 -> 30001, 19088743
func bigLiteral(x int) int {
	print(x + 30000)
	print((x + 0x123456789) >> 8)
	return 0
}

// print only printed the low 32 bits, sign-extended (it did
// 'addw a0, a0, zero').
// x = 3000000000 -> 3000000000
func printBig(x int) int {
	print(x)
	return 0
}
