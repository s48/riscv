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
