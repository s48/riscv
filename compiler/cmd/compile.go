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
	name       string
	entryPoint string
	inputs     []uint64
	outputs    []uint64
	exitStatus uint8
}

var tests = []*testT{
	{"fib", "fib", []uint64{19}, []uint64{6765}, 0},
	{"add", "add", []uint64{3, 4}, []uint64{7}, 0},
	{"sub", "sub", []uint64{10, 3}, []uint64{7}, 0},
	{"bitXor", "bitXor", []uint64{12, 10}, []uint64{6}, 0},
	{"bitOr", "bitOr", []uint64{12, 10}, []uint64{14}, 0},
	{"bitAnd", "bitAnd", []uint64{12, 10}, []uint64{8}, 0},
	{"logOr", "logOr", []uint64{3, 5}, []uint64{0, 1, 1, 1}, 0},
	{"logAnd", "logAnd", []uint64{3, 5}, []uint64{0, 0, 0, 1}, 0},
	{"not", "not", []uint64{3, 5}, []uint64{0, 1}, 0},
	{"eq", "eq", []uint64{3, 5}, []uint64{0, 1, 0}, 0},
	{"neq", "neq", []uint64{3, 5}, []uint64{1, 0, 1}, 0},
	{"lt", "lt", []uint64{3, 5}, []uint64{1, 0, 0}, 0},
	{"leq", "leq", []uint64{3, 5}, []uint64{1, 1, 0}, 0},
	{"gt", "gt", []uint64{3, 5}, []uint64{0, 0, 1}, 0},
	{"geq", "geq", []uint64{3, 5}, []uint64{0, 1, 1}, 0},
}

var gdbServer = flag.Bool("gdb", false, "Run GDB server.")
var testCase = flag.String("test", "", "Test to run.")
var writeObjectFile = flag.Bool("objectfile", false, "Write object file.")

func main() {
	flag.Parse()
	for _, test := range tests {
		if *testCase == "" || *testCase == test.name {
			runTest(test)
		}
	}
}

func runTest(test *testT) (status bool) {
	/*
		defer func() {
			if err := recover(); err != nil {
				fmt.Printf("test '%s' panicked: %v\n", test.name, err)
				status = false
			}
		}()
	*/
	objectFile := ""
	if *writeObjectFile {
		objectFile = test.name + ".o"
	}
	fmt.Printf("\nRunning test '%s'\n", test.name)
	cpu, inputs := compiler.CompileModule("tests", test.entryPoint, objectFile)
	for i, input := range test.inputs {
		if inputs[i] != 0 {
			cpu.Registers[inputs[i]] = input
		}
	}
	if *gdbServer {
		emulator.GdbServer(cpu)
	} else {
		cpu.Run()
	}
	status = true
	if cpu.ExitStatus != test.exitStatus {
		fmt.Printf("exit status %d instead of %d\n", cpu.ExitStatus, test.exitStatus)
		status = false
	}
	if !slices.Equal(cpu.Output, test.outputs) {
		fmt.Printf("Output is %v instead of %v.\n", cpu.Output, test.outputs)
		status = false
	}
	return status
}
