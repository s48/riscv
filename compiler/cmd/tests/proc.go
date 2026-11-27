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

// Arrays are values, so assigning one copies it.  Each check changes
// the original after the copy, which must not change the copy.
// n = 1 -> 11, 22, 0, 12, 7, 63
func arrayCopy(n int) int {
	var a [3][2]int
	for i := range 3 {
		a[i][0] = i*10 + 1
		a[i][1] = i*10 + 2
	}
	a[0] = a[n] // copy in place
	a[n][0] = 0
	print(a[0][0])
	row := a[2] // copy to a new variable
	a[2][1] = 0
	print(row[1])
	b := a[0]
	b = a[n] // copy in place, to a variable
	a[n][1] = 3
	print(b[0])
	print(b[1])
	var c [2]int
	setFirst(c, 5) // the argument is copied
	c[0] = 7
	print(c[0])
	print(arraySum(a[2]))
	return 0
}

func setFirst(a [2]int, x int) {
	a[0] = x
}

func arraySum(a [2]int) int {
	return a[0] + a[1] + a[0] + a[0]
}

// Copies too large for straight-line code use the library routines,
// with 8-byte pieces for 'a' and single bytes for 'b'.  'c' is
// copied in 2-byte pieces.
// n = 5 -> 50, 5, 3, 0, 0, 0
func arrayCopyBig(n int) int {
	var a [20]int
	var b [30]uint8
	var c [3]int16
	for i := range 20 {
		a[i] = i * 10
	}
	for i := range 30 {
		b[i] = uint8(i)
	}
	c[2] = 3
	a2 := a
	b2 := b
	c2 := c
	a[n] = 0
	b[n] = 0
	c[2] = 0
	print(a2[n])
	print(int(b2[n]))
	print(int(c2[2]))
	print(a[n])
	print(int(b[n]))
	print(int(c[2]))
	return 0
}

// A pointer to an array is the array's data.  Assigning to the array
// through the pointer copies into it.  The last check assigns through
// a pointer to an int.
// n = 1 -> 20, 7, 30, 7, 5
func arrayPointer(n int) int {
	var a [3]int
	var b [3]int
	p := &a
	(*p)[n] = 20
	print(a[n])
	a[n] = 7
	print((*p)[n])
	b[n] = 30
	*p = b
	print(a[n])
	b[n] = 0
	a = *p
	print(arraySum3(*p, n) - 23)
	x := 0
	q := &x
	*q = 5
	print(x)
	return 0
}

func arraySum3(a [3]int, n int) int {
	a[0] = 0
	return a[n]
}

// Pointers to an array's elements, both arrays and ints, and to the
// whole array.
// n = 1 -> 5, 9, 4, 5, 6
func arrayAddress(n int) int {
	var a [3][2]int
	p := &a[n]
	(*p)[0] = 5
	print(a[n][0])
	q := &a[2][n]
	*q = 9
	print(a[2][1])
	r := &a
	(*r)[0][1] = 4
	print(a[0][1])
	row := *p // a copy
	a[n][0] = 6
	print(row[0])
	print((*p)[0])
	return 0
}

// Ranging over arrays, with and without values, over a pointer to an
// array, and over an array of arrays.  The values come from a copy of
// the array, so changing it in the loop doesn't change them, but the
// values from a pointer are not copied.
// n = 2 -> 3, 60, 60, 90, 21
func arrayRange(n int) int {
	var a [3]int
	for i := range a {
		a[i] = (i + 1) * 10
	}
	count := 0
	for range a {
		count++
	}
	print(count)
	sum := 0
	for i, v := range a {
		a[2] = 100 // not seen by v
		sum += v
		a[i] = v
	}
	print(sum)
	a[2] = 30
	sum = 0
	for _, v := range &a {
		sum += v
	}
	print(sum)
	sum = 0
	for i, v := range &a {
		if i == 0 {
			a[n] = 60 // seen by v
		}
		sum += v
	}
	print(sum)
	var b [2][3]int
	b[1][n] = 21
	last := 0
	for _, row := range b {
		last = row[n]
	}
	print(last)
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
