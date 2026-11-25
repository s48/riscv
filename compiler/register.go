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
	// For now only the callee-saved registers, s0-s11, are allocated.
	// Values that are live across calls are then preserved.
	intRegsMask = 1<<8 | 1<<9 | 0x3FF<<18
	numArgRegs  = 8 // a0-a7
)

var r0 *registerT      // 0
var rReturn *registerT // 1, the return address
var rStack *registerT  // 2
var rGlobal *registerT // 3
var rTemp *registerT   // 5, t0, for use within a call's code
// These are special for system calls.
var rArg0 *registerT
var rArg7 *registerT

var intRegisters = &cps.RegisterClassT{Name: "i"}
var intRegs = make([]cps.RegisterT, numIntRegs)

func reg(i int) *registerT { return intRegs[i].(*registerT) }

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
	intRegisters.SetRegisters(intRegs, intRegsMask)
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
