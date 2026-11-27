// Copyright 2026 Richard Kelsey. All rights reserved.
// See file LICENSE for notices and license.

package compiler

import (
	"fmt"
	"go/constant"
	"go/token"
	"math/bits"
	"slices"

	"github.com/s48/transform/cps"
)

type instT struct {
	bits   uint32
	pos    token.Pos
	callTo *cps.CallNodeT // procedure for a 'jal' that has to be filled in
}

type codeGeneratorT struct {
	allocation *cps.RegAllocationT
	frame      *frameT // for the procedure being generated
	usesHeap   bool    // some call allocates, so the heap prelude is needed
}

func makeCodeGenerator(allocation *cps.RegAllocationT) *codeGeneratorT {
	return &codeGeneratorT{allocation: allocation}
}

// The frame has the spill slots, if there are any, one more for use by
// parallelMove, and the callee-saved registers that the procedure uses.
type frameT struct {
	size        int // in bytes, zero if there is no frame
	scratchSlot int
	saved       []cps.RegisterT
	savedSlot   int // slot of the first saved register
}

// The entry point never returns, so it doesn't need to save registers.
func (generator *codeGeneratorT) setProc(proc *cps.CallNodeT, isEntry bool) {
	allocation := generator.allocation.Frames[proc]
	frame := &frameT{
		scratchSlot: allocation.NumSlots,
		savedSlot:   allocation.NumSlots,
	}
	if 0 < allocation.NumSlots {
		frame.savedSlot += 1 // the scratch slot is only needed if there are slots
	}
	if !isEntry {
		for _, register := range allocation.Registers {
			if isCalleeSaved(register.Number()) {
				frame.saved = append(frame.saved, register)
			}
		}
	}
	if 0 < allocation.NumSlots || 0 < len(frame.saved) {
		frame.size = ((frame.savedSlot+len(frame.saved))*8 + 15) &^ 15 // keep the stack aligned
	}
	generator.frame = frame
}

func (generator *codeGeneratorT) GenerateCall(call *cps.CallNodeT) ([]instT, int) {
	insts := &riscvInstsT{frame: generator.frame}
	moves := generator.allocation.Moves[call]
	if moves != nil {
		parallelMove(insts, moves.Before)
		parallelMove(insts, moves.At)
	}
	call.Primop.(riscvPrimopT).generate(insts, call)
	generator.usesHeap = generator.usesHeap || insts.usesHeap
	result := addSource(insts.instructions, call.Source())
	if insts.callTo != nil {
		result[len(result)-1].callTo = insts.callTo
	}
	return result, len(result)
}

func addSource(insts []uint32, pos token.Pos) []instT {
	result := make([]instT, len(insts))
	for i, bits := range insts {
		result[i] = instT{bits: bits, pos: pos}
	}
	return result
}

func (generator *codeGeneratorT) JumpSize(
	call *cps.CallNodeT,
	offset int,
	offsets []int,
) int {
	return call.Primop.(riscvJumpPrimopT).jumpSize(call, offset, offsets)
}

func (generator *codeGeneratorT) GenerateJump(
	call *cps.CallNodeT,
	offset int,
	offsets []int,
	jumpSize int,
) []instT {
	insts := &riscvInstsT{}
	call.Primop.(riscvJumpPrimopT).generateJump(insts, call, offset, offsets, jumpSize)
	return addSource(insts.instructions, call.Source())
}

type riscvPrimopT interface {
	cps.PrimopT
	generate(insts *riscvInstsT, call *cps.CallNodeT)
}

type riscvJumpPrimopT interface {
	jumpSize(call *cps.CallNodeT, offset int, offsets []int) int // have upper bounds on offsets
	// Offsets are known.  This must add jumpSize instructions.
	generateJump(insts *riscvInstsT, call *cps.CallNodeT, offset int, offsets []int, jumpSize int)
}

func addPrimop(primop cps.PrimopT) {
	cps.PrimopTable[primop.Name()] = primop
}

func init() {
	// core CPS primops
	addPrimop(&procLambdaPrimopT{})
	addPrimop(&procCallPrimopT{})
	addPrimop(&returnPrimopT{})
	addPrimop(&jumpLambdaPrimopT{})
	addPrimop(&jumpPrimopT{})
	addPrimop(&ifPrimopT{})
	addPrimop(&ifEqPrimopT{})
	addPrimop(&ifLtPrimopT{})
	addPrimop(&compPrimopT{cps.CompPrimopT{"<", "if<", false}})
	addPrimop(&compPrimopT{cps.CompPrimopT{">=", "if<", true}})
	addPrimop(&compPrimopT{cps.CompPrimopT{"==", "if=", false}})
	addPrimop(&compPrimopT{cps.CompPrimopT{"!=", "if=", true}})
	addPrimop(&letPrimopT{})
	addPrimop(&letrecPrimopT{})
	addPrimop(&makeCellPrimopT{})
	addPrimop(&cellRefPrimopT{})
	addPrimop(&cellSetPrimopT{})
	addPrimop(&pointerRefPrimopT{})
	addPrimop(&pointerSetPrimopT{})
	addPrimop(&addressOfPrimopT{})
	addPrimop(&arrayIndexPrimopT{})
	addPrimop(&newPrimopT{})
	addPrimop(&defaultValuePrimopT{})
	addPrimop(&loadRegPrimopT{})
	addPrimop(&syscallPrimopT{})
}

// These inherit simplifiers and other methods from the compiler core.

type procLambdaPrimopT struct{ cps.ProcLambdaPrimopT }

// The continuation is in ra and the arguments in a0-a7.
func (primop *procLambdaPrimopT) RegisterUsage(call *cps.CallNodeT) (regUsesT, regUsesT) {
	outputs := make(regUsesT, len(call.Outputs))
	outputs[0] = contOut
	for i := range outputs[1:] {
		outputs[i+1] = argSpec(i, cps.LateRegUse)
	}
	return nil, outputs
}
func (primop *procLambdaPrimopT) generate(insts *riscvInstsT, call *cps.CallNodeT) {
	frame := insts.frame
	if frame.size != 0 {
		insts.addImm(rStack, rStack, -frame.size)
	}
	for i, register := range frame.saved {
		insts.store(register, rStack, (frame.savedSlot+i)*8)
	}
}

type procCallPrimopT struct{ cps.ProcCallPrimopT }

func (primop *procCallPrimopT) RegisterUsage(call *cps.CallNodeT) (regUsesT, regUsesT) {
	inputs := make(regUsesT, len(call.Inputs))
	for i := range inputs[1:] { // no register needed for the procedure
		inputs[i+1] = argSpec(i, cps.EarlyRegUse)
	}
	outputs := make(regUsesT, len(call.Outputs))
	for i := range outputs {
		outputs[i] = argSpec(i, cps.LateRegUse)
	}
	return inputs, outputs
}

type returnPrimopT struct{ cps.ReturnPrimopT }

func (primop *returnPrimopT) RegisterUsage(call *cps.CallNodeT) (regUsesT, regUsesT) {
	inputs := make(regUsesT, len(call.Inputs))
	tuple := call.InputVariable(0).Type.(*nativeTupleT)
	inputs[0] = contIn // return pointer
	for i := range tuple.contents {
		inputs[i+1] = argSpec(i, cps.EarlyRegUse)
	}
	return inputs, nil
}

// The return values and the continuation have already been moved to a0-a7
// and ra.
func (primop *returnPrimopT) generate(insts *riscvInstsT, call *cps.CallNodeT) {
	frame := insts.frame
	for i, register := range frame.saved {
		insts.load(rStack, (frame.savedSlot+i)*8, register)
	}
	if frame.size != 0 {
		insts.addImm(rStack, rStack, frame.size)
	}
	insts.iFormat(0, rReturn, 0, r0, 0x67) // jalr zero, 0(ra)
}

func (primop *procCallPrimopT) ClobberedRegisters(call *cps.CallNodeT) []cps.RegisterT {
	return callerSaved
}

// The arguments have already been moved to a0-a7.  The offset in the 'jal'
// is filled in once the code for all procedures has been laid out.
func (primop *procCallPrimopT) generate(insts *riscvInstsT, call *cps.CallNodeT) {
	insts.jFormat(0, rReturn, 0x6f)
	insts.callTo = cps.CalledLambda(call)
}

type jumpLambdaPrimopT struct{ cps.JumpLambdaPrimopT }

func (primop *jumpLambdaPrimopT) RegisterUsage(call *cps.CallNodeT) (regUsesT, regUsesT) {
	return nil, lambdaOutRegSpecs(call)
}
func (primop *jumpLambdaPrimopT) generate(insts *riscvInstsT, call *cps.CallNodeT) {}

type jumpPrimopT struct{ cps.JumpPrimopT }

type ifPrimopT struct{ cps.IfPrimopT }

type letPrimopT struct{ cps.LetPrimopT }
type letrecPrimopT struct{ cps.LetrecPrimopT }

type makeCellPrimopT struct{ cps.MakeCellPrimopT }
type cellRefPrimopT struct{ cps.CellRefPrimopT }
type cellSetPrimopT struct{ cps.CellSetPrimopT }

type pointerRefPrimopT struct{ cps.PointerRefPrimopT }
type pointerSetPrimopT struct{ cps.PointerSetPrimopT }

// A cell is a single 64-bit word allocated in the heap.  The thread
// object, pointed to by rThread, holds the heap pointer at offset 0 and
// the heap limit at offset 8.  See heapPrelude in compiler.go.
const (
	threadHeapPointer = 0
	threadHeapLimit   = 8
	cellSize          = 8
)

func (primop *makeCellPrimopT) RegisterUsage(call *cps.CallNodeT) (regUsesT, regUsesT) {
	return nil, uses(intOut)
}

func (primop *makeCellPrimopT) generate(insts *riscvInstsT, call *cps.CallNodeT) {
	insts.allocate(call.Outputs[0], cellSize)
}

// Bump-allocate 'size' bytes, rounded up to a multiple of 8, leaving
// the address in 'out'.  'out' holds the size and then the limit until
// the result is written to it.  There is no GC yet, so running out of
// heap traps with an ebreak.  The heap is never reused, so the new
// object is all zeros, as the emulator's memory starts out zeroed.
func (insts *riscvInstsT) allocate(out any, size int) {
	size = (size + 7) &^ 7
	insts.load(rThread, threadHeapPointer, rTemp)
	if signedLen(size) <= 12 {
		insts.addImm(rTemp, rTemp, size)
	} else {
		insts.loadImmediate(size, out)
		insts.rFormat(0, out, rTemp, 0, rTemp, 0x33) // add
	}
	insts.load(rThread, threadHeapLimit, out)
	insts.branch(7, out, rTemp, 8)    // bgeu: skip the ebreak if limit >= new pointer
	insts.iFormat(1, r0, 0, r0, 0x73) // ebreak
	insts.store(rTemp, rThread, threadHeapPointer)
	if signedLen(-size) <= 12 {
		insts.addImm(rTemp, out, -size)
	} else {
		insts.loadImmediate(size, out)
		insts.rFormat(0x20, out, rTemp, 0, out, 0x33) // sub
	}
	insts.usesHeap = true
}

func (primop *cellRefPrimopT) RegisterUsage(call *cps.CallNodeT) (regUsesT, regUsesT) {
	return uses(intIn), uses(intOut)
}
func (primop *cellRefPrimopT) generate(insts *riscvInstsT, call *cps.CallNodeT) {
	generateDeref(insts, call)
}

func (primop *pointerRefPrimopT) RegisterUsage(call *cps.CallNodeT) (regUsesT, regUsesT) {
	return uses(intIn), uses(intOut)
}

// The load is the size of the value, and sign- or zero-extends it.
// A cell holds a 64-bit word, which is fine, as the low bytes of an
// extended value are the value.
//
// An array value is a pointer to its data, and an array within an
// array is stored in place, so the inner array's value is its address.
func (primop *pointerRefPrimopT) generate(insts *riscvInstsT, call *cps.CallNodeT) {
	typ := call.Outputs[0].Type.(nativeTypeT)
	if typ.kind() == arrayType {
		if regNumber(call.Inputs[0]) != regNumber(call.Outputs[0]) {
			insts.move(call.Inputs[0], call.Outputs[0])
		}
		return
	}
	insts.loadSized(call.Inputs[0], 0, call.Outputs[0], typ.sizeBytes(), isSignedType(typ))
}

// Load the value the first input points to.
func generateDeref(insts *riscvInstsT, call *cps.CallNodeT) {
	insts.load(call.Inputs[0], 0, call.Outputs[0])
}

func (primop *pointerSetPrimopT) RegisterUsage(call *cps.CallNodeT) (regUsesT, regUsesT) {
	return uses(intIn, intIn), nil
}

// pointerSet(pointer, value), storing only the value's bytes.
func (primop *pointerSetPrimopT) generate(insts *riscvInstsT, call *cps.CallNodeT) {
	typ := nodeType(call.Inputs[1])
	if typ.kind() == arrayType {
		panicf("storing an array value, %s, requires copying it, which is not supported", typ)
	}
	size := typ.sizeBytes()
	insts.storeSized(call.Inputs[1], call.Inputs[0], 0, size)
}

// cellSet(cell, value)
func (primop *cellSetPrimopT) RegisterUsage(call *cps.CallNodeT) (regUsesT, regUsesT) {
	return uses(intIn, intIn), nil
}
func (primop *cellSetPrimopT) generate(insts *riscvInstsT, call *cps.CallNodeT) {
	insts.store(call.Inputs[1], call.Inputs[0], 0)
}

type compPrimopT struct{ cps.CompPrimopT }

func (primop *compPrimopT) RegisterUsage(call *cps.CallNodeT) (regUsesT, regUsesT) {
	return uses(intIn, intIn), nil
}

type ifLtPrimopT struct{ cps.IfLtPrimopT }

func (primop *ifLtPrimopT) RegisterUsage(call *cps.CallNodeT) (regUsesT, regUsesT) {
	return uses(intIn, intIn), nil
}

func (primop *ifLtPrimopT) generate(insts *riscvInstsT, call *cps.CallNodeT) {}

func (ifLtPrimopT) jumpSize(call *cps.CallNodeT, offset int, offsets []int) int {
	return branchJumpSize(call, offset, offsets)
}

func (ifLtPrimopT) generateJump(
	insts *riscvInstsT,
	call *cps.CallNodeT,
	offset int,
	offsets []int,
	jumpSize int,
) {
	branchGenerateJump(insts, call, offset, offsets, jumpSize, 0x4, 0x5)
}

type ifEqPrimopT struct{ cps.IfEqPrimopT }

func (primop *ifEqPrimopT) RegisterUsage(call *cps.CallNodeT) (regUsesT, regUsesT) {
	return uses(intIn, intIn), nil
}

func (primop *ifEqPrimopT) generate(insts *riscvInstsT, call *cps.CallNodeT) {}

func (ifEqPrimopT) jumpSize(call *cps.CallNodeT, offset int, offsets []int) int {
	return branchJumpSize(call, offset, offsets)
}

func (ifEqPrimopT) generateJump(
	insts *riscvInstsT,
	call *cps.CallNodeT,
	offset int,
	offsets []int,
	jumpSize int,
) {
	branchGenerateJump(insts, call, offset, offsets, jumpSize, 0x0, 0x1)
}

func branchJumpSize(call *cps.CallNodeT, offset int, offsets []int) int {
	if offsets[0] == 0 || offsets[1] == 0 {
		return 1 // one arm is a fall through, so just a branch
	} else {
		return 2 // branch plus jump
	}
}

func branchGenerateJump(
	insts *riscvInstsT,
	call *cps.CallNodeT,
	offset int,
	offsets []int,
	jumpSize int,
	imm3Forward uint32,
	imm3Reverse uint32,
) {
	x := call.Inputs[0]
	y := call.Inputs[1]
	if offsets[0] == 0 {
		insts.bFormat((offsets[1]-offset)*4, y, x, imm3Reverse, 0x63)
	} else {
		insts.bFormat((offsets[0]-offset)*4, y, x, imm3Forward, 0x63)
		if offsets[1] != 0 {
			insts.jFormat((offsets[1]-(offset+1))*4, r0, 0x6f)
		}
	}
}

// Primops that don't generate code and/or don't assign registers.
func (primop *letPrimopT) generate(insts *riscvInstsT, call *cps.CallNodeT)          {}
func (primop *letPrimopT) RegisterUsage(call *cps.CallNodeT) (regUsesT, regUsesT)    { return nil, nil }
func (primop *letrecPrimopT) generate(insts *riscvInstsT, call *cps.CallNodeT)       {}
func (primop *letrecPrimopT) RegisterUsage(call *cps.CallNodeT) (regUsesT, regUsesT) { return nil, nil }

// The nil at the start is for the variable bound to the jump target.
func (primop *jumpPrimopT) RegisterUsage(call *cps.CallNodeT) (regUsesT, regUsesT) {
	inputs := append(regUsesT{nil}, lambdaInRegSpecs(cps.CalledLambda(call))...)
	return inputs, nil
}

func lambdaInRegSpecs(lambda *cps.CallNodeT) regUsesT {
	specs := make(regUsesT, len(lambda.Outputs))
	for i := range specs {
		specs[i] = intIn // all we have at this point
	}
	return specs
}

func lambdaOutRegSpecs(lambda *cps.CallNodeT) regUsesT {
	specs := make(regUsesT, len(lambda.Outputs))
	for i := range specs {
		specs[i] = intOut // all we have at this point
	}
	return specs
}

// The moves for the arguments are done by the code generator.
func (primop *jumpPrimopT) generate(insts *riscvInstsT, call *cps.CallNodeT) {}

func (primop *jumpPrimopT) jumpSize(call *cps.CallNodeT, offset int, offsets []int) int {
	if offsets[0] == 0 {
		return 0 // fall through
	} else {
		return 1 // jump
	}
}

func (primop *jumpPrimopT) generateJump(
	insts *riscvInstsT,
	call *cps.CallNodeT,
	offset int,
	offsets []int,
	jumpSize int,
) {
	if offsets[0] != 0 {
		insts.jFormat((offsets[0]-offset)*4, r0, 0x6f)
	}
}

// Algorithm from:
//   Tilting at windmills with Coq: formal verification of a
//   compilation algorithm for parallel moves
//   Laurence Rideau, Bernard P. Serpette, Xavier Leroy
//   https://hal.inria.fr/inria-00289709/document

// Could have an initial phase where unobstructed moves
// are done and the locations are changed to the destination.
// If anything is left then move something to a temp and
// repeat, at which point the thing in the temp can go in
// its destination (is this guaranteed, or would it be
// better to find the loop?).  Then repeat the inital
// phase.

const (
	inSource = iota // value is in its 'from' register
	inDest          // value is in its 'to' register
	pending         // value is pending in an earlier recursive call
)

// Moving from a slot or a constant to a slot uses rTemp.  If there are any of these
// then loops have to be broken using the extra slot.

func parallelMove(insts *riscvInstsT, moves []cps.RegMoveT) {
	count := len(moves)
	froms := make([]cps.RegLocationT, count)
	tos := make([]cps.RegLocationT, count)
	locations := make([]int, count)
	temp := cps.RegLocationT{Register: rTemp}
	for i, move := range moves {
		froms[i] = move.From
		tos[i] = move.To
		locations[i] = inSource
		if move.From.Register == nil && move.To.Register == nil {
			temp = cps.RegLocationT{Slot: insts.frame.scratchSlot}
		}
	}
	// Recursive function that moves the 'i'th value to its target
	// location.
	var move func(int)
	move = func(i int) {
		if froms[i] == tos[i] {
			return
		}
		locations[i] = pending
		for j := range count {
			if froms[j] == tos[i] {
				if locations[j] == inSource {
					move(j)
				} else if locations[j] == pending {
					// Found a loop, put this in temp.
					insts.moveLocation(froms[j], temp)
					froms[j] = temp
				}
			}
		}
		insts.moveLocation(froms[i], tos[i])
		locations[i] = inDest
	}
	for i := range count {
		if locations[i] == inSource {
			move(i)
		}
	}
}

// A narrow literal is wrapped to its type's size, as constant folding
// does not (uint8 200 + 100 folds to 300).
func (insts *riscvInstsT) loadConstant(literal cps.NodeT, to cps.RegisterT) {
	value := valueAsInt(literal)
	typ, isNative := literal.(*cps.LiteralNodeT).Type.(nativeTypeT)
	if isNative && typ.sizeBytes() < 8 {
		value = wrapInt(value, typ.sizeBytes(), isSignedType(typ))
	}
	insts.loadImmediate(value, to)
}

// Load any 64-bit value into 'dest', using only 'dest'.
//   - 12 bits: addi dest, zero, value
//   - 32 bits: lui dest, hi20; addiw dest, dest, lo12.  addiw, not
//     addi, as hi20 may be 0x80000 (lui gives -2^31), and the 32-bit
//     wraparound then gives the right value.
//   - otherwise: load the value without its low 12 bits, shifted right
//     as far as its trailing zeros allow, then slli and addi lo12.
//
// lo12 is the sign-extended low 12 bits, so the part above it is
// 'value - lo12', which may round up.
func (insts *riscvInstsT) loadImmediate(value int, dest any) {
	if signedLen(value) <= 12 {
		insts.addImm(r0, dest, value)
		return
	}
	lo12 := (value << 52) >> 52
	if signedLen(value) <= 32 {
		hi20 := (value - lo12) >> 12
		insts.lui(uint32(hi20&0xFFFFF), dest)
		if lo12 != 0 {
			insts.iFormat(lo12, dest, 0, dest, 0x1B) // addiw
		}
		return
	}
	hi := (value - lo12) >> 12
	shift := 12 + bits.TrailingZeros64(uint64(hi))
	insts.loadImmediate((value-lo12)>>shift, dest)
	insts.iFormat(shift, dest, 1, dest, 0x13) // slli
	if lo12 != 0 {
		insts.addImm(dest, dest, lo12)
	}
}

// Sign- or zero-extend the low 'numBytes' of 'value'.
func wrapInt(value int, numBytes int, signed bool) int {
	shift := 64 - 8*numBytes
	if signed {
		return (value << shift) >> shift
	}
	return int(uint(value<<shift) >> shift)
}

func (insts *riscvInstsT) moveLocation(from cps.RegLocationT, to cps.RegLocationT) {
	if from.Register != nil && to.Register != nil {
		insts.move(from.Register, to.Register)
	} else if from.Constant != nil && to.Register != nil {
		insts.loadConstant(from.Constant, to.Register)
	} else if from.Constant != nil {
		insts.loadConstant(from.Constant, rTemp)
		insts.store(rTemp, rStack, to.Slot*8)
	} else if from.Register != nil {
		insts.store(from.Register, rStack, to.Slot*8)
	} else if to.Register != nil {
		insts.load(rStack, from.Slot*8, to.Register)
	} else {
		insts.load(rStack, from.Slot*8, rTemp)
		insts.store(rTemp, rStack, to.Slot*8)
	}
}

type loadRegPrimopT struct{ cps.LoadRegPrimopT }

func (primop *loadRegPrimopT) RegisterUsage(call *cps.CallNodeT) ([]*cps.RegUseSpecT, []*cps.RegUseSpecT) {
	return nil, uses(intOut)
}

func (primop *loadRegPrimopT) generate(insts *riscvInstsT, call *cps.CallNodeT) {
	// If it isn't in a register here then it is loaded where it is needed.
	register := call.Outputs[0].Register()
	if register != nil {
		insts.loadConstant(call.Inputs[0], register)
	}
}

type syscallPrimopT struct{}

func (primop *syscallPrimopT) Name() string                 { return "syscall" }
func (primop *syscallPrimopT) SideEffects() bool            { return true }
func (primop *syscallPrimopT) Simplify(call *cps.CallNodeT) { cps.DefaultSimplify(call) }

// The value is passed in a0 and the call number in a7.
func (primop *syscallPrimopT) RegisterUsage(call *cps.CallNodeT) (regUsesT, regUsesT) {
	return uses(nil, nil, argSpec(0, cps.EarlyRegUse)), nil
}

func (primop *syscallPrimopT) ClobberedRegisters(call *cps.CallNodeT) []cps.RegisterT {
	return []cps.RegisterT{rArg0, rArg7}
}

// The value has already been moved to a0.
func (primop *syscallPrimopT) generate(insts *riscvInstsT, call *cps.CallNodeT) {
	insts.iFormat(valueAsInt(call.Inputs[1]), r0, 0, rArg7, 0x13)
	insts.iFormat(0, r0, 0, r0, 0x73) // ecall
}

//----------------------------------------------------------------

func init() {
	addPrimop(&notPrimopT{})
}

type notPrimopT struct{}

func (primop *notPrimopT) Name() string                 { return "!" }
func (primop *notPrimopT) SideEffects() bool            { return false }
func (primop *notPrimopT) Simplify(call *cps.CallNodeT) { simplifyNot(call) }
func (primop *notPrimopT) RegisterUsage(call *cps.CallNodeT) (regUsesT, regUsesT) {
	return uses(intIn), uses(intOut)
}

func (primop *notPrimopT) generate(insts *riscvInstsT, call *cps.CallNodeT) {
	insts.iFormat(
		1, // imm
		call.Inputs[0],
		0x4, // xor
		call.Outputs[0],
		0x13)
}

// 1. !true -> false, !false -> true
// 2. !!A -> A
// 3. if !A then C0 else C1 -> if A then C1 else C0
func simplifyNot(call *cps.CallNodeT) {
	arg := call.Inputs[0]
	if cps.IsLiteralNode(arg) {
		literal := arg.(*cps.LiteralNodeT)
		value := literal.Constant()
		if value.Kind() != constant.Bool {
			panic(fmt.Sprintf("notPrimop has non-boolean argument '%s'", value.ExactString()))
		}
		literal.Value = constant.MakeBool(!constant.BoolVal(value))
		call.Primop = cps.LookupPrimop("let")
		cps.MarkChanged(call)
		return
	}
	cps.SimplifyNext(call)
	refs := slices.Clone(call.Outputs[0].Refs)
	for _, ref := range refs {
		parent := ref.Parent()
		if parent.Primop.Name() == "!" {
			for _, ref := range parent.Outputs[0].Refs {
				cps.ReplaceInput(ref, cps.CopyNodeTree(arg))
			}
		} else if parent.Primop.Name() == "if" {
			cps.ReplaceInput(parent.Inputs[0], cps.CopyNodeTree(arg))
			temp := cps.DetachNext(parent.Next[0])
			cps.AttachNext(parent, cps.DetachNext(parent.Next[1]), 0)
			cps.AttachNext(parent, temp, 1)
		}
	}
	if len(call.Outputs[0].Refs) == 0 {
		cps.RemoveCall(call)
	}
}

type addressOfPrimopT struct{}

func (primop *addressOfPrimopT) Name() string                 { return "addressOf" }
func (primop *addressOfPrimopT) SideEffects() bool            { return false }
func (primop *addressOfPrimopT) Simplify(call *cps.CallNodeT) { cps.DefaultSimplify(call) }
func (primop *addressOfPrimopT) RegisterUsage(call *cps.CallNodeT) (regUsesT, regUsesT) {
	return nil, nil
}
func (primop *addressOfPrimopT) generate(insts *riscvInstsT, call *cps.CallNodeT) {
}

type arrayIndexPrimopT struct{}

func (primop *arrayIndexPrimopT) Name() string                 { return "arrayIndex" }
func (primop *arrayIndexPrimopT) SideEffects() bool            { return false }
func (primop *arrayIndexPrimopT) Simplify(call *cps.CallNodeT) { cps.DefaultSimplify(call) }
func (primop *arrayIndexPrimopT) RegisterUsage(call *cps.CallNodeT) (regUsesT, regUsesT) {
	return uses(intIn, intIn), uses(intOut)
}

// arrayIndex(array, index) returns the address of array[index].  An
// index out of range (including a negative one, which is a large
// unsigned value) traps with an ebreak.  The output may be in the same
// register as either input, so it is written only by the last
// instruction.
func (primop *arrayIndexPrimopT) generate(insts *riscvInstsT, call *cps.CallNodeT) {
	array := nodeArrayType(call.Inputs[0])
	index := call.Inputs[1]
	out := call.Outputs[0]
	insts.loadImmediate(array.length, rTemp)
	insts.branch(6, index, rTemp, 8)  // bltu: skip the ebreak if index < length
	insts.iFormat(1, r0, 0, r0, 0x73) // ebreak
	elementSize := array.contents.sizeBytes()
	if elementSize == 1 {
		insts.rFormat(0, index, call.Inputs[0], 0, out, 0x33) // add
		return
	}
	if bits.OnesCount(uint(elementSize)) == 1 {
		shift := bits.TrailingZeros(uint(elementSize))
		insts.iFormat(shift, index, 1, rTemp, 0x13) // slli
	} else {
		// An element that is itself an array may be any size.
		insts.loadImmediate(elementSize, rTemp)
		insts.rFormat(0x01, rTemp, index, 0, rTemp, 0x33) // mul
	}
	insts.rFormat(0, rTemp, call.Inputs[0], 0, out, 0x33) // add
}

type newPrimopT struct{}

func (primop *newPrimopT) Name() string                 { return "new" }
func (primop *newPrimopT) SideEffects() bool            { return false }
func (primop *newPrimopT) Simplify(call *cps.CallNodeT) { cps.DefaultSimplify(call) }
func (primop *newPrimopT) RegisterUsage(call *cps.CallNodeT) (regUsesT, regUsesT) {
	return nil, uses(intOut)
}

// Allocate the object for an array.  The elements are stored in place,
// including those that are themselves arrays.
func (primop *newPrimopT) generate(insts *riscvInstsT, call *cps.CallNodeT) {
	array, isArray := call.Outputs[0].Type.(*nativeArrayT)
	if !isArray {
		panicf("new of non-array type %s", call.Outputs[0].Type)
	}
	insts.allocate(call.Outputs[0], array.sizeBytes())
}

// The native type of a reference or literal.
func nodeType(node cps.NodeT) nativeTypeT {
	switch node := node.(type) {
	case *cps.ReferenceNodeT:
		return node.Variable.Type.(nativeTypeT)
	case *cps.LiteralNodeT:
		return node.Type.(nativeTypeT)
	}
	panicf("node %v has no type", node)
	return nil
}

func nodeArrayType(node cps.NodeT) *nativeArrayT {
	array, isArray := nodeType(node).(*nativeArrayT)
	if !isArray {
		panicf("%v is not an array", node)
	}
	return array
}

// defaultValuePrimopT creates the default value for its output.  For
// non-heap types this is a literal (bound to a variable via 'let'),
// for heap types it is a call to 'new'.
//
// This is a macro primop, in that its ToCps method does not add a
// call to itself.

type defaultValuePrimopT struct{}

func (primop *defaultValuePrimopT) Name() string                 { return "defaultValue" }
func (primop *defaultValuePrimopT) SideEffects() bool            { return false }
func (primop *defaultValuePrimopT) Simplify(call *cps.CallNodeT) {}
func (primop *defaultValuePrimopT) RegisterUsage(call *cps.CallNodeT) (regUsesT, regUsesT) {
	return nil, nil
}
func (primop *defaultValuePrimopT) generate(insts *riscvInstsT, call *cps.CallNodeT) {}

func (primop *defaultValuePrimopT) ToCps(
	args []cps.NodeT,
	resultVars []*cps.VariableT,
	calls *cps.CallsT) {

	nativeType := convertType(resultVars[0].Type)
	var call *cps.CallNodeT
	if nativeType.kind() == arrayType {
		call = cps.MakeCall(cps.LookupPrimop("new"), resultVars)
	} else {
		value := cps.MakeLiteral(int64(0), nativeType)
		call = cps.MakeCall(cps.LookupPrimop("let"), resultVars, value)
	}
	calls.AppendCall(call)
}

//----------------------------------------------------------------

func init() {
	addPrimop(&printPrimopT{})
}

type printPrimopT struct{}

func (primop *printPrimopT) Name() string                 { return "print" }
func (primop *printPrimopT) SideEffects() bool            { return true }
func (primop *printPrimopT) Simplify(call *cps.CallNodeT) { cps.DefaultSimplify(call) }

// The value is passed in a0.
func (primop *printPrimopT) RegisterUsage(call *cps.CallNodeT) (regUsesT, regUsesT) {
	return uses(argSpec(0, cps.EarlyRegUse)), nil
}

func (primop *printPrimopT) ClobberedRegisters(call *cps.CallNodeT) []cps.RegisterT {
	return []cps.RegisterT{rArg0, rArg7}
}

// The value has already been moved to a0.
func (primop *printPrimopT) generate(insts *riscvInstsT, call *cps.CallNodeT) {
	insts.iFormat(1, r0, 0, rArg7, 0x13) // 1 = PrintInteger
	insts.iFormat(0, r0, 0, r0, 0x73)    // ecall
}

//----------------------------------------------------------------

func valueAsInt(node cps.NodeT) int {
	value := node.(*cps.LiteralNodeT).Constant()
	typ := node.(*cps.LiteralNodeT).Type
	if typ == theBoolType {
		if constant.BoolVal(value) {
			return 1
		} else {
			return 0
		}
	} else {
		signed, exact := constant.Int64Val(value)
		if exact {
			return int(signed)
		}
		unsigned, exact := constant.Uint64Val(value)
		if exact {
			return int(unsigned)
		}
		panic("literal node's value is not an int: " + value.ExactString())
	}
}
