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

func branch(x int, y int) int {
	if x < y {
		x += 1
	} else {
		x -= 1
	}
	if x < y {
		x += 1
	} else {
		x -= 1
	}
	print(x)
	return 0
}

// Nested loops.  'scale' is used only in the inner loop, 'y' only in the
// outer one (other than to limit the inner one), and 'offset' in neither.
func nested(x int, y int) int {
	scale := x + y
	offset := x - y
	sum := 0
	for i := range x {
		row := 0
		for j := range y {
			row += (i + j) * scale
		}
		sum += row - y
	}
	print(sum)
	print(sum + offset)
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

func square(x int) int {
	return x * x
}

func calls(x int, y int) int {
	print(square(x) + square(y))
	return 0
}

// Recursion, with 'n' live across the call.
func fact(n int) int {
	if n < 2 {
		return 1
	}
	return n * fact(n-1)
}

func recursive(n int) int {
	print(fact(n))
	print(fact(n + 1))
	return 0
}

func divmod(x int, y int) (int, int) {
	return x / y, x % y
}

func sum8(a int, b int, c int, d int, e int, f int, g int, h int) int {
	return a + 2*b + 3*c + 4*d + 5*e + 6*f + 7*g + 8*h
}

// Multiple results, many arguments, constant arguments, calls in a loop,
// and calls as arguments.
func manyCalls(x int, y int) int {
	q, r := divmod(x, y)
	print(q)
	print(r)
	total := 0
	for i := range y {
		q, r = divmod(x+i, y)
		total += q*10 + r
	}
	print(total)
	print(sum8(1, 2, 3, 4, 5, 6, 7, 8))
	print(sum8(x, y, q, r, total, x, y, fact(4)))
	print(fact(fact(3)))
	return 0
}

// More values live across a call than there are callee-saved registers.
func pressure(x int, y int) int {
	a := x + 1
	b := x + 2
	c := x + 3
	d := x + 4
	e := x + 5
	f := x + 6
	g := x + 7
	h := x + 8
	i := x + 9
	j := x + 10
	k := x + 11
	l := x + 12
	m := x + 13
	n := x + 14
	z := fact(y)
	print(a + b + c + d + e + f + g + h + i + j + k + l + m + n + z)
	print(a*b - c*d + e*f - g*h + i*j - k*l + m*n)
	return 0
}

func cells(x int, y int) int {
	var z *int
	if x < y {
		z = &x
	} else {
		z = &y
	}
	x += 10
	y += 20
	print(*z)
	return 0
}
