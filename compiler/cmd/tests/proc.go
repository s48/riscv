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

func array(n int) int {
	var a [4]int
	for i := range 4 {
		a[i] = i*10 + 10
	}
	print(a[n])
	return 0
}

// Narrow elements are stored in their own size, and sign- or
// zero-extended when loaded.  Each store would overwrite the next
// element if they were 64 bits.
// n = 1 -> -56, 200, -1, 65535, -100000, 4000000000
func arrayNarrow(n int) int {
	var a [3]int8
	var b [3]uint8
	var c [3]int16
	var d [3]uint16
	var e [3]int32
	var f [3]uint32
	for i := range 3 {
		a[i] = int8(n * 200)
		b[i] = uint8(n * 200)
		c[i] = int16(-n)
		d[i] = uint16(-n)
		e[i] = int32(n * -100000)
		f[i] = uint32(n * 4000000000)
	}
	a[n+1] = 0
	b[n+1] = 0
	c[n+1] = 0
	d[n+1] = 0
	e[n+1] = 0
	f[n+1] = 0
	print(int(a[n]))
	print(int(b[n]))
	print(int(c[n]))
	print(int(d[n]))
	print(int(e[n]))
	print(int(f[n]))
	return 0
}

// Arrays of arrays, with inner arrays whose sizes are a power of two
// (4 * 8) and not (3 * 2).
// n = 2 -> 21, 12, -21, 1
func arrayNested(n int) int {
	var a [3][4]int
	var b [4][3]int16
	for i := range 3 {
		for j := range 4 {
			a[i][j] = i*10 + j
			b[j][i] = int16(-(i*10 + j))
		}
	}
	print(a[n][n-1])
	print(a[n-1][n])
	print(int(b[n-1][n]))
	print(a[0][1])
	return 0
}

// An array too big for an addi immediate, and two arrays, to check
// that they don't overlap.
// n = 299 -> 2990, 299, 0
func arrayBig(n int) int {
	var a [300]int
	var b [300]int
	for i := range 300 {
		a[i] = i * 10
		b[i] = i
	}
	print(a[n])
	print(b[n])
	print(a[0])
	return 0
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
