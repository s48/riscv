// Copyright 2026 Richard Kelsey. All rights reserved.
// See file LICENSE for notices and license.

package tests

func fib(x int) int {
	x0 := 0
	x1 := 1
	for range x {
		temp := x0 + x1
		x0 = x1
		x1 = temp
	}
	print(x1)
	return 0
}

func add(x int, y int) int {
	print(x + y)
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

func logOr(x int, y int) int {
	r0 := 0
	if y < x || y < x { // false || false
		r0 = 1
	}
	r1 := 0
	if x < y || y < x { // true || false
		r1 = 1
	}
	r2 := 0
	if y < x || x < y { // false || true
		r2 = 1
	}
	r3 := 0
	if x < y || x < y { // true || true
		r3 = 1
	}
	print(r0)
	print(r1)
	print(r2)
	print(r3)
	return 0
}

func logAnd(x int, y int) int {
	r0 := 0
	if y < x && y < x { // false && false
		r0 = 1
	}
	r1 := 0
	if x < y && y < x { // true && false
		r1 = 1
	}
	r2 := 0
	if y < x && x < y { // false && true
		r2 = 1
	}
	r3 := 0
	if x < y && x < y { // true && true
		r3 = 1
	}
	print(r0)
	print(r1)
	print(r2)
	print(r3)
	return 0
}

func not(x int, y int) int {
	// Test constant folding into 'if' expressions.
	r0 := 0
	if !(x < y) { // !true
		r0 = 1
	}
	r1 := 0
	if !(y < x) { // !false
		r1 = 1
	}
	// Test constant folding for !.
	r2 := 0
	if !true {
		r2 = 1
	}
	r3 := 0
	if !!true {
		r3 = 1
	}
	// Test generating an actual run-time '!'.
	b := false
	if x < y {
		b = !b
	}
	print(r0)
	print(r1)
	print(r2)
	print(r3)
	print(b)
	return 0
}

func eq(x int, y int) int {
	r0 := 0
	if x == y {
		r0 = 1
	}
	r1 := 0
	if x == x {
		r1 = 1
	}
	r2 := 0
	if y == x {
		r2 = 1
	}
	print(r0)
	print(r1)
	print(r2)
	return 0
}

func neq(x int, y int) int {
	r0 := 0
	if x != y {
		r0 = 1
	}
	r1 := 0
	if x != x {
		r1 = 1
	}
	r2 := 0
	if y != x {
		r2 = 1
	}
	print(r0)
	print(r1)
	print(r2)
	return 0
}

func lt(x int, y int) int {
	r0 := 0
	if x < y {
		r0 = 1
	}
	r1 := 0
	if x < x {
		r1 = 1
	}
	r2 := 0
	if y < x {
		r2 = 1
	}
	print(r0)
	print(r1)
	print(r2)
	return 0
}

func leq(x int, y int) int {
	r0 := 0
	if x <= y {
		r0 = 1
	}
	r1 := 0
	if x <= x {
		r1 = 1
	}
	r2 := 0
	if y <= x {
		r2 = 1
	}
	print(r0)
	print(r1)
	print(r2)
	return 0
}

func gt(x int, y int) int {
	r0 := 0
	if x > y {
		r0 = 1
	}
	r1 := 0
	if x > x {
		r1 = 1
	}
	r2 := 0
	if y > x {
		r2 = 1
	}
	print(r0)
	print(r1)
	print(r2)
	return 0
}

func geq(x int, y int) int {
	r0 := 0
	if x >= y {
		r0 = 1
	}
	r1 := 0
	if x >= x {
		r1 = 1
	}
	r2 := 0
	if y >= x {
		r2 = 1
	}
	print(r0)
	print(r1)
	print(r2)
	return 0
}
