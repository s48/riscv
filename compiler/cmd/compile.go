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
	{"fib", []uint64{19}, []uint64{6765}, 0},
	{"branch", []uint64{1, 5}, []uint64{3}, 0},
	{"nested", []uint64{5, 4}, []uint64{610, 611}, 0},
	{"calls", []uint64{3, 4}, []uint64{25}, 0},
	{"recursive", []uint64{5}, []uint64{120, 720}, 0},
	{"manyCalls", []uint64{7, 3}, []uint64{2, 1, 73, 204, 642, 720}, 0},
	{"pressure", []uint64{5, 4}, []uint64{199, 204}, 0},
	{"cells", []uint64{5, 4}, []uint64{24}, 0},
	{"add", []uint64{3, 4}, []uint64{7}, 0},
	{"sub", []uint64{10, 3}, []uint64{7}, 0},
	{"bitXor", []uint64{12, 10}, []uint64{6}, 0},
	{"bitOr", []uint64{12, 10}, []uint64{14}, 0},
	{"bitAnd", []uint64{12, 10}, []uint64{8}, 0},
	{"mul", []uint64{6, 7}, []uint64{42}, 0},
	{"div", []uint64{17, 5}, []uint64{3}, 0},
	{"rem", []uint64{17, 5}, []uint64{2}, 0},
	{"shl", []uint64{3, 4}, []uint64{48}, 0},
	{"shr", []uint64{100, 2}, []uint64{25}, 0},
	{"logOr", []uint64{3, 5}, []uint64{0, 1, 1, 1}, 0},
	{"logAnd", []uint64{3, 5}, []uint64{0, 0, 0, 1}, 0},
	{"not", []uint64{3, 5}, []uint64{0, 1, 0, 1, 1}, 0},
	{"eq", []uint64{3, 5}, []uint64{0, 1, 0}, 0},
	{"neq", []uint64{3, 5}, []uint64{1, 0, 1}, 0},
	{"lt", []uint64{3, 5}, []uint64{1, 0, 0}, 0},
	{"leq", []uint64{3, 5}, []uint64{1, 1, 0}, 0},
	{"gt", []uint64{3, 5}, []uint64{0, 0, 1}, 0},
	{"geq", []uint64{3, 5}, []uint64{0, 1, 1}, 0},

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
	{"addConst", nil, []uint64{5}, 7},
	{"subConst", nil, []uint64{7}, 7},
	{"bitXorConst", nil, []uint64{6}, 7},
	{"bitOrConst", nil, []uint64{14}, 7},
	{"bitAndConst", nil, []uint64{8}, 7},
	{"mulConst", nil, []uint64{42}, 7},
	{"divConst", nil, []uint64{3}, 7},
	{"shlConst", nil, []uint64{16}, 7},
	{"shrConst", nil, []uint64{8}, 7},
	{"remConst", nil, []uint64{2}, 7},
}

var gdbServer = flag.Bool("gdb", false, "Run GDB server.")
var testCase = flag.String("test", "", "Test to run.")
var writeObjectFile = flag.Bool("objectfile", false, "Write object file.")

func main() {
	flag.Parse()
	if *testCase != "" {
		var theTest *testT
		for _, test := range tests {
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
