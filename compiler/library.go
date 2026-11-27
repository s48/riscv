// Copyright 2026 Richard Kelsey. All rights reserved.
// See file LICENSE for notices and license.

package compiler

import (
	"github.com/s48/transform/cps"
)

// Library routines are added to the end of the code when they are
// called, much as the heap prelude is added to the start.  Until
// there are separate object files and linking, this is how code that
// needs a loop, which a primop's straight-line code can't have, is
// shared.
//
// They do not use the standard calling convention.  Like a system
// call, each has its arguments in fixed registers and changes only
// the registers that it lists, plus rTemp.  They are called with
// 'jal ra', so ra is changed as well.

type libraryRoutineT struct {
	name     string
	generate func(insts *riscvInstsT)
}

// In the order in which they appear in the code.
var libraryRoutines = []libraryRoutineT{
	{"copyBytes", func(insts *riscvInstsT) { generateCopyLoop(insts, 1) }},
	{"copyDoublewords", func(insts *riscvInstsT) { generateCopyLoop(insts, 8) }},
}

// Call a library routine.  The offset is filled in when the code is
// laid out.
func (insts *riscvInstsT) callRoutine(name string) {
	insts.jFormat(0, rReturn, 0x6f) // jal ra, <name>
	insts.routine = name
}

// The copy routines copy a2 pieces, each 'piece' bytes, from a1 to a0.
// a2 must be greater than zero.  a0, a1, and a2 are changed.
func copyRoutineClobbers() []cps.RegisterT {
	return []cps.RegisterT{rReturn, reg(10), reg(11), reg(12)}
}

func generateCopyLoop(insts *riscvInstsT, piece int) {
	dest, src, count := reg(10), reg(11), reg(12)
	insts.loadSized(src, 0, rTemp, piece, false)
	insts.storeSized(rTemp, dest, 0, piece)
	insts.addImm(src, src, piece)
	insts.addImm(dest, dest, piece)
	insts.addImm(count, count, -1)
	insts.branch(1, count, r0, -20)        // bne: loop while count != 0
	insts.iFormat(0, rReturn, 0, r0, 0x67) // jalr zero, 0(ra)
}
