// Copyright 2026 Richard Kelsey. All rights reserved.
// See file LICENSE for notices and license.

package tests

// Tests for known bugs.  Their entries in compile.go's test table are
// commented out.  When a bug is fixed, its test moves to regression.go
// and its entry is enabled.

// Constant folding through narrow types.  This prints the right value,
// as loadConstant wraps the folded 300 to 44, but convertInt does not
// fold (see the comment on convertIntPrimopT.Simplify), leaving a move.
// Like the ...Const tests in arith.go, this should compile to 6
// instructions.
// -> 44
func convConst() int {
	var z uint8 = 200
	z = z + 100
	print(int(z))
	return 0
}
