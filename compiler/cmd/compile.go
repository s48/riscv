// Copyright 2026 Richard Kelsey. All rights reserved.
// See file LICENSE for notices and license.

package main

import (
	"flag"
	"fmt"
	"slices"

	"github.com/s48/riscv/compiler"
	"github.com/s48/riscv/emulator"
)

type testT struct {
	name         string
	inputs       []uint64
	outputs      []uint64
	instructions int // expected instruction count, or 0 to not check it
}

var tests = []*testT{
	// proc.go
	{"fib", []uint64{19}, []uint64{6765}, 0},
	{"branch", []uint64{1, 5}, []uint64{3}, 0},
	{"nested", []uint64{5, 4}, []uint64{610, 611}, 0},
	{"calls", []uint64{3, 4}, []uint64{25}, 0},
	{"recursive", []uint64{5}, []uint64{120, 720}, 0},
	{"manyCalls", []uint64{7, 3}, []uint64{2, 1, 73, 204, 642, 720}, 0},
	{"pressure", []uint64{5, 4}, []uint64{199, 204}, 0},
	{"cells", []uint64{5, 4}, []uint64{24}, 0},
	{"array", []uint64{1}, []uint64{20}, 0},
	{"arrayNarrow", []uint64{1}, []uint64{uint64(-56 & 0xFFFFFFFFFFFFFFFF), 200, uint64(-1 & 0xFFFFFFFFFFFFFFFF), 65535, uint64(-100000 & 0xFFFFFFFFFFFFFFFF), 4000000000}, 0},
	{"arrayNested", []uint64{2}, []uint64{21, 12, uint64(-21 & 0xFFFFFFFFFFFFFFFF), 1}, 0},
	{"arrayBig", []uint64{299}, []uint64{2990, 299, 0}, 0},
	{"logOr", []uint64{3, 5}, []uint64{0, 1, 1, 1}, 0},
	{"logAnd", []uint64{3, 5}, []uint64{0, 0, 0, 1}, 0},
	{"not", []uint64{3, 5}, []uint64{0, 1, 0, 1, 1}, 0},
	{"eq", []uint64{3, 5}, []uint64{0, 1, 0}, 0},
	{"neq", []uint64{3, 5}, []uint64{1, 0, 1}, 0},
	{"lt", []uint64{3, 5}, []uint64{1, 0, 0}, 0},
	{"leq", []uint64{3, 5}, []uint64{1, 1, 0}, 0},
	{"gt", []uint64{3, 5}, []uint64{0, 0, 1}, 0},
	{"geq", []uint64{3, 5}, []uint64{0, 1, 1}, 0},

	// arith.go
	{"add", []uint64{3, 4}, []uint64{7}, 0},
	{"add8", []uint64{100}, []uint64{uint64(-56 & 0xFFFFFFFFFFFFFFFF)}, 0},
	{"add8u", []uint64{200}, []uint64{144}, 0},
	{"sub", []uint64{10, 3}, []uint64{7}, 0},
	{"bitXor", []uint64{12, 10}, []uint64{6}, 0},
	{"bitOr", []uint64{12, 10}, []uint64{14}, 0},
	{"bitAnd", []uint64{12, 10}, []uint64{8}, 0},
	{"mul", []uint64{6, 7}, []uint64{42}, 0},
	{"div", []uint64{17, 5}, []uint64{3}, 0},
	{"rem", []uint64{17, 5}, []uint64{2}, 0},
	{"shl", []uint64{3, 4}, []uint64{48}, 0},
	{"shr", []uint64{100, 2}, []uint64{25}, 0},
	{"negate", []uint64{3}, []uint64{uint64(-3 & 0xFFFFFFFFFFFFFFFF)}, 0},
	{"bitNot", []uint64{12}, []uint64{uint64(-13 & 0xFFFFFFFFFFFFFFFF)}, 0},
	{"unaryNarrow", []uint64{5}, []uint64{uint64(-5 & 0xFFFFFFFFFFFFFFFF), 251, uint64(-6 & 0xFFFFFFFFFFFFFFFF), 250}, 0},
	{"unaryNarrowConst", nil, []uint64{uint64(-5 & 0xFFFFFFFFFFFFFFFF), 251, uint64(-6 & 0xFFFFFFFFFFFFFFFF), 250}, 0},
	{"narrowWrap", []uint64{100}, []uint64{1, 44, 36, 5}, 0},
	{"bigConsts", nil, []uint64{2047, 18446744073709549568, 2048, 18446744073709549567, 2147481600, 2147483647, 18446744071562067968, 2147483648, 4294967295, 1311768467463790320, 9223372036854775807, 9223372036854775808, 18446744073709551615, 9223372036854777856}, 0},
	{"convInt8", []uint64{200}, []uint64{18446744073709551560, 16777215, 200, 18446744073709551560, 65480, 18446744073709551560, 16777215, 18446744073709551560, 16777215}, 0},
	{"convUint8", []uint64{200}, []uint64{144, 18446744073709551504, 144, 144, 144, 144, 144, 144}, 0},
	{"convInt16", []uint64{40000}, []uint64{18446744073709526080, 16777215, 64, 64, 40000, 18446744073709526080, 16777116, 18446744073709526080, 16777215}, 0},
	{"convUint16", []uint64{40000}, []uint64{14464, 18446744073709551488, 128, 14464, 14464, 14464, 18446744073709526080, 40000, 40000}, 0},
	{"convInt32", []uint64{3000000200}, []uint64{18446744072414584520, 16777215, 18446744073709551560, 200, 24264, 24264, 11718750, 18446744072414584520, 16777215}, 0},
	{"convUint32", []uint64{3000000200}, []uint64{1705033104, 18446744073709551504, 144, 18446744073709534608, 48528, 1705033104, 18446744072414584520, 11718750, 11718750}, 0},
	{"convInt64", []uint64{0x7FFFFFFFFFFFFF80, 0x7FFFFFFFFFFFFF80}, []uint64{18446744073709551488, 128, 18446744073709551488, 65408, 18446744073709551488, 16777215, 8388607, 18446744073709551488, 128, 18446744073709551488, 65408, 18446744073709551488, 16777215, 8388607}, 0},

	// Constant-folding tests: each of these takes no inputs and
	// computes its result entirely from literals.  If the compiler
	// actually folds the operation at compile time, the only
	// arithmetic instruction needed is the one to load the
	// already-computed answer ("li"); if folding stops working, an
	// extra instruction appears to load the operands and compute the
	// result at run time instead.  "instructions" pins down the
	// exact count (checked against CompileModule's own instruction
	// count, not just by eyeballing a listing), so a regression that
	// silently stops folding -- but still prints the right answer --
	// fails the test anyway.
	{"addConst", nil, []uint64{5}, 6},
	{"subConst", nil, []uint64{7}, 6},
	{"bitXorConst", nil, []uint64{6}, 6},
	{"bitOrConst", nil, []uint64{14}, 6},
	{"bitAndConst", nil, []uint64{8}, 6},
	{"mulConst", nil, []uint64{42}, 6},
	{"divConst", nil, []uint64{3}, 6},
	{"shlConst", nil, []uint64{16}, 6},
	{"shrConst", nil, []uint64{8}, 6},
	{"remConst", nil, []uint64{2}, 6},
	{"negateConst", nil, []uint64{uint64(-5 & 0xFFFFFFFFFFFFFFFF)}, 6},
	{"bitNotConst", nil, []uint64{uint64(-13 & 0xFFFFFFFFFFFFFFFF)}, 6},

	// regression.go: bugs that have been fixed.
	{"negVar", []uint64{3}, []uint64{7}, 0},
	{"negConst", []uint64{100}, []uint64{44}, 0},
	{"printNarrow", []uint64{200}, []uint64{uint64(-56 & 0xFFFFFFFFFFFFFFFF)}, 0},
	{"narrowEq", []uint64{200}, []uint64{1, 0}, 0},
	{"narrowLt", []uint64{200}, []uint64{1, 1}, 0},
	{"narrowDiv", []uint64{200}, []uint64{72, 4}, 0},
	{"narrowShr", []uint64{200}, []uint64{72}, 0},
	{"bigLiteral", []uint64{1}, []uint64{30001, 19088743}, 0},
	{"printBig", []uint64{3000000000}, []uint64{3000000000}, 0},
}

// Tests for the known bugs in bug.go.  These are run only when asked
// for by name with -test.  When a bug is fixed, its test moves to
// regression.go and its entry to the table above.
var bugTests = []*testT{
	{"convConst", nil, []uint64{44}, 6},
}

var gdbServer = flag.Bool("gdb", false, "Run GDB server.")
var testCase = flag.String("test", "", "Test to run.")
var writeObjectFile = flag.Bool("objectfile", false, "Write object file.")

func main() {
	flag.Parse()
	if *testCase != "" {
		var theTest *testT
		for _, test := range slices.Concat(tests, bugTests) {
			if test.name == *testCase {
				theTest = test
				break
			}
		}
		if theTest == nil {
			fmt.Printf("Test '%s' not found.\n", *testCase)
		} else {
			result := runTest(theTest, true)
			fmt.Printf("%s\n", result)
		}
	} else {
		successes := ""
		results := make([]string, len(tests))
		for i, test := range tests {
			result := runTest(test, false)
			if result == "success" {
				successes += " " + test.name
			} else {
				results[i] = result
			}
		}
		fmt.Printf("\n")
		if successes != "" {
			fmt.Printf("Passed:%s\n", successes)
		}
		for i, test := range tests {
			if results[i] != "" {
				fmt.Printf("%s: %s\n", test.name, results[i])
			}
		}
	}
}

func runTest(test *testT, panic bool) (result string) {
	if !panic {
		defer func() {
			if err := recover(); err != nil {
				result = fmt.Sprintf("test '%s' panicked: %v", test.name, err)
			}
		}()
	}
	objectFile := ""
	if *writeObjectFile {
		objectFile = test.name + ".o"
	}
	fmt.Printf("\nRunning test '%s'\n", test.name)
	cpu, inputs, instCount := compiler.CompileModule("tests", test.name, objectFile)
	for i, input := range test.inputs {
		if inputs[i] != 0 {
			cpu.Registers[inputs[i]] = input
		}
	}
	if 0 < test.instructions && instCount != test.instructions {
		return fmt.Sprintf("compiled to %d instructions instead of %d.", instCount, test.instructions)
	}
	if *gdbServer {
		emulator.GdbServer(cpu)
	} else {
		cpu.Run()
	}
	//	if cpu.ExitStatus != test.exitStatus {
	//		return fmt.Sprintf("exit status %d instead of %d.", cpu.ExitStatus, test.exitStatus)
	//	}
	if !slices.Equal(cpu.Output, test.outputs) {
		return fmt.Sprintf("Output is %v instead of %v.", cpu.Output, test.outputs)
	}
	return "success"
}
