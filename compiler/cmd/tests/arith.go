// Copyright 2026 Richard Kelsey. All rights reserved.
// See file LICENSE for notices and license.

package tests

func add(x int, y int) int {
	print(x + y)
	return 0
}

func add8(x int) int {
	var z int8 = 100
	z = z + int8(x)
	print(int(z))
	return 0
}

func add8u(x uint) uint {
	var z uint8 = 200
	z = z + uint8(x)
	print(uint(z))
	return 0
}

// The ...Const functions take no inputs.  Their purpose is to test
// constant folding.  The test verifies that the object code has
// minimal size, as nothing is computed, as well as producing the
// correct result.

func addConst() int {
	print(2 + 3)
	return 0
}

func sub(x int, y int) int {
	print(x - y)
	return 0
}

func subConst() int {
	print(10 - 3)
	return 0
}

func bitXor(x int, y int) int {
	print(x ^ y)
	return 0
}

func bitXorConst() int {
	print(12 ^ 10)
	return 0
}

func bitOr(x int, y int) int {
	print(x | y)
	return 0
}

func bitOrConst() int {
	print(12 | 10)
	return 0
}

func bitAnd(x int, y int) int {
	print(x & y)
	return 0
}

func bitAndConst() int {
	print(12 & 10)
	return 0
}

func mul(x int, y int) int {
	print(x * y)
	return 0
}

func mulConst() int {
	print(6 * 7)
	return 0
}

func div(x int, y int) int {
	print(x / y)
	return 0
}

func divConst() int {
	print(17 / 5)
	return 0
}

func rem(x int, y int) int {
	print(x % y)
	return 0
}

func remConst() int {
	print(17 % 5)
	return 0
}

func negate(x int) int {
	print(-x)
	return 0
}

// The front end has already folded constant expressions like '-5', so
// these use a variable that is bound to a constant.
func negateConst() int {
	x := 5
	print(-x)
	return 0
}

func bitNot(x int) int {
	print(^x)
	return 0
}

func bitNotConst() int {
	x := 12
	print(^x)
	return 0
}

// x = 5 -> -5, 251, -6, 250
func unaryNarrow(x int) int {
	a := int8(x)
	b := uint8(x)
	print(int(-a))
	print(uint(-b))
	print(int(^a))
	print(uint(^b))
	return 0
}

// convertInt does not fold yet (see convConst in regression.go), so
// these are not minimal.
// -> -5, 251, -6, 250
func unaryNarrowConst() int {
	var a int8 = 5
	var b uint8 = 5
	print(int(-a))
	print(uint(-b))
	print(int(^a))
	print(uint(^b))
	return 0
}

func shl(x int, y int) int {
	print(x << y)
	return 0
}

func shlConst() int {
	print(2 << 3)
	return 0
}

func shr(x int, y int) int {
	print(x >> y)
	return 0
}

func shrConst() int {
	print(64 >> 3)
	return 0
}

// Conversions from each size to every other.  The inputs are chosen
// so that the narrowed value has its sign bit set, and some tests
// add the value to itself first so that there are carries into the
// bits above it.  Results that would not fit in 32 bits are shifted
// down, as print only prints the low 32 bits (see regression.go).

// x = 200, so int8(x) is -56.
func convInt8(x int) int {
	a := int8(x)
	print(int(a))
	print(uint(a) >> 40)
	print(int(uint8(a)))
	print(int(int16(a)))
	print(int(uint16(a)))
	print(int(int32(a)))
	print(int(uint32(a)) >> 8)
	print(int(int64(a)))
	print(uint64(a) >> 40)
	return 0
}

// x = 200, so a + a carries into bit 8.
func convUint8(x int) int {
	a := uint8(x)
	a = a + a
	print(int(a))
	print(int(int8(a)))
	print(int(int16(a)))
	print(int(uint16(a)))
	print(int(int32(a)))
	print(int(uint32(a)))
	print(int(int64(a)))
	print(int(uint64(a)))
	return 0
}

// x = 40000, so int16(x) is -25536.
func convInt16(x int) int {
	a := int16(x)
	print(int(a))
	print(uint(a) >> 40)
	print(int(int8(a)))
	print(int(uint8(a)))
	print(int(uint16(a)))
	print(int(int32(a)))
	print(int(uint32(a)) >> 8)
	print(int(int64(a)))
	print(uint64(a) >> 40)
	return 0
}

// x = 40000, so a + a carries into bit 16 and int16(x) is negative.
func convUint16(x int) int {
	a := uint16(x)
	a = a + a
	print(int(a))
	print(int(int8(a)))
	print(int(uint8(a)))
	print(int(int16(a)))
	print(int(int32(a)))
	print(int(uint32(a)))
	b := uint16(x)
	print(int(int16(b)))
	print(int(int32(b)))
	print(int(uint64(b)))
	return 0
}

// x = 3000000200, so int32(x) is -1294967096 and int8(x) is -56.
func convInt32(x int) int {
	a := int32(x)
	print(int(a))
	print(uint(a) >> 40)
	print(int(int8(a)))
	print(int(uint8(a)))
	print(int(int16(a)))
	print(int(uint16(a)))
	print(int(uint32(a)) >> 8)
	print(int(int64(a)))
	print(uint64(a) >> 40)
	return 0
}

// x = 3000000200, so a + a carries into bit 32 and int32(x) is negative.
func convUint32(x int) int {
	a := uint32(x)
	a = a + a
	print(int(a))
	print(int(int8(a)))
	print(int(uint8(a)))
	print(int(int16(a)))
	print(int(uint16(a)))
	print(int(int32(a)))
	b := uint32(x)
	print(int(int32(b)))
	print(int(b) >> 8)
	print(int(uint64(b)) >> 8)
	return 0
}

// x = 0x7FFFFFFF_FFFFFF80 (signed 64 bits down to every size), y the
// same, as a uint.
func convInt64(x int, y uint) int {
	print(int(int8(x)))
	print(int(uint8(x)))
	print(int(int16(x)))
	print(int(uint16(x)))
	print(int(int32(x)))
	print(int(uint32(x)) >> 8)
	print(uint64(x) >> 40)
	print(int(int8(y)))
	print(int(uint8(y)))
	print(int(int16(y)))
	print(int(uint16(y)))
	print(int(int32(y)))
	print(int(uint32(y)) >> 8)
	print(int64(y) >> 40)
	return 0
}
