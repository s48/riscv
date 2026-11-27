// Copyright 2026 Richard Kelsey. All rights reserved.
// See file LICENSE for notices and license.

package compiler

import (
	"strconv"

	"github.com/s48/transform/cps"
)

type registerT struct {
	class  *cps.RegisterClassT
	number int
	name   string
}

func (reg *registerT) Class() *cps.RegisterClassT { return reg.class }
func (reg *registerT) SetClass(class *cps.RegisterClassT) {
	reg.class = class
}
func (reg *registerT) Number() int    { return reg.number }
func (reg *registerT) String() string { return reg.name }

func makeRegister(class *cps.RegisterClassT, number int) *registerT {
	return &registerT{class, number, class.Name[:1] + strconv.Itoa(number)}
}

const (
	numIntRegs = 32
	numArgRegs = 8 // a0-a7
)

var r0 *registerT      // 0
var rReturn *registerT // 1, the return address
var rStack *registerT  // 2
var rGlobal *registerT // 3
var rThread *registerT // 4
var rTemp *registerT   // 5, t0, for use within a call's code
// These are special for system calls.
var rArg0 *registerT
var rArg7 *registerT

var intRegisters = &cps.RegisterClassT{Name: "i"}
var intRegs = make([]cps.RegisterT, numIntRegs)

func reg(i int) *registerT { return intRegs[i].(*registerT) }

// t1-t6, a0-a7, s0-s11
var allocatedRegisters = []int{
	6, 7, 28, 29, 30, 31,
	10, 11, 12, 13, 14, 15, 16, 17,
	8, 9, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27,
}

// s0-s11, which are preserved by procedure calls.
func isCalleeSaved(i int) bool {
	return i == 8 || i == 9 || (18 <= i && i <= 27)
}

// Registers whose values are changed by procedure calls: ra, t0-t6, a0-a7.
var callerSaved = []cps.RegisterT{}

var intIn = &cps.RegUseSpecT{PhaseOffset: cps.EarlyRegUse, Class: intRegisters}
var intOut = &cps.RegUseSpecT{PhaseOffset: cps.LateRegUse, Class: intRegisters}

type regUsesT = []*cps.RegUseSpecT

var gdbRegNames = []string{
	"zero",
	"ra",             // return address
	"sp",             // stack pointer
	"gp",             // global pointer
	"tp",             // thread pointer
	"t0", "t1", "t2", // temps
	"fp", // frame pointer / save 0
	"s1", // save 1
	// arguments
	"a0", "a1", "a2", "a3", "a4", "a5", "a6", "a7",
	// more saves
	"s2", "s3", "s4", "s5", "s6", "s7", "s8", "s9", "s10", "s11",
	// more temps
	"t3", "t4", "t5", "t6",
}

func init() {
	for i := range intRegs {
		intRegs[i] = &registerT{intRegisters, i, "i" + strconv.Itoa(i)}
	}
	// The registers that are allocated, in order of preference.  Callee-saved
	// registers come last as using them requires saving and restoring them.
	// Zero, ra, sp, gp, tp, and t0 are not allocated.
	allocated := []cps.RegisterT{}
	for _, i := range allocatedRegisters {
		allocated = append(allocated, intRegs[i])
	}
	intRegisters.SetRegisters(allocated, 1<<len(allocated)-1)
	for i := range numIntRegs {
		if !isCalleeSaved(i) && i != 0 && i != 2 && i != 3 && i != 4 {
			callerSaved = append(callerSaved, intRegs[i])
		}
	}
	for i, name := range gdbRegNames {
		reg(i).name = name
	}
	r0 = reg(0)
	rReturn = reg(1)
	rTemp = reg(5)
	contIn = &cps.RegUseSpecT{PhaseOffset: cps.EarlyRegUse, Class: intRegisters, Fixed: rReturn}
	contOut = &cps.RegUseSpecT{PhaseOffset: cps.LateRegUse, Class: intRegisters, Fixed: rReturn}
	rStack = reg(2)
	rGlobal = reg(3)
	rThread = reg(4)
	rArg0 = reg(10)
	rArg7 = reg(17)
}

// Specs for values that are passed in a0-a7, with continuations in ra.
func argSpec(i int, phase int) *cps.RegUseSpecT {
	if numArgRegs <= i {
		panic("too many arguments or return values, only 8 are supported")
	}
	return &cps.RegUseSpecT{PhaseOffset: phase, Class: intRegisters, Fixed: reg(10 + i)}
}

// These are set in init().
var contIn *cps.RegUseSpecT
var contOut *cps.RegUseSpecT

func uses(uses ...*cps.RegUseSpecT) regUsesT {
	return uses
}
