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
	numIntRegs  = 32
	intRegsMask = 0xFFFFFFF0 // 0b1111 are reserved
)

var r0 *registerT      // 0
var rTemp *registerT   // 1
var rStack *registerT  // 2
var rGlobal *registerT // 3
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
	rTemp = reg(1)
	rStack = reg(2)
	rGlobal = reg(3)
	rArg0 = reg(10)
	rArg7 = reg(17)
}

func uses(uses ...*cps.RegUseSpecT) regUsesT {
	return uses
}
