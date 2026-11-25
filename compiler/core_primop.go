// Copyright 2026 Richard Kelsey. All rights reserved.
// See file LICENSE for notices and license.

package compiler

import (
	"fmt"
	"go/constant"
	"go/token"
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
}

func makeCodeGenerator(allocation *cps.RegAllocationT) *codeGeneratorT {
	return &codeGeneratorT{allocation: allocation}
}

// The frame has the spill slots, one more for use by parallelMove, and
// the callee-saved registers that the procedure uses.
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
		saved:       allocation.Registers,
		savedSlot:   allocation.NumSlots + 1,
	}
	if isEntry {
		frame.saved = nil
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

func (insts *riscvInstsT) loadConstant(literal cps.NodeT, to cps.RegisterT) {
	insts.iFormat(valueAsInt(literal), r0, 0, to, 0x13)
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

// Shoot.  Reg specs are going to need a mask, not just a class.
// For now just juggle as needed.
func (primop *syscallPrimopT) RegisterUsage(call *cps.CallNodeT) (regUsesT, regUsesT) {
	//	return uses(nil, arg7, arg0), nil
	return uses(nil, nil, intIn), nil
}

func (primop *syscallPrimopT) generate(insts *riscvInstsT, call *cps.CallNodeT) {
	syscall := valueAsInt(call.Inputs[1])
	arg0 := call.Inputs[2]
	// Save a7 and a0.
	insts.addImm(rStack, rStack, -16)
	insts.store(rArg7, rStack, 0)
	insts.store(rArg0, rStack, 8)
	// Load arguments and make the call.
	insts.iFormat(syscall, r0, 0, rArg7, 0x13)
	insts.move(arg0, rArg0)
	insts.iFormat(0, r0, 0, r0, 0x73)
	// Restore a7 and a0.
	insts.load(rStack, 0, rArg7)
	insts.load(rStack, 8, rArg0)
	insts.addImm(rStack, rStack, 16)
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

//----------------------------------------------------------------

func init() {
	addPrimop(&printPrimopT{})
}

type printPrimopT struct{}

func (primop *printPrimopT) Name() string                 { return "print" }
func (primop *printPrimopT) SideEffects() bool            { return true }
func (primop *printPrimopT) Simplify(call *cps.CallNodeT) { cps.DefaultSimplify(call) }
func (primop *printPrimopT) RegisterUsage(call *cps.CallNodeT) (regUsesT, regUsesT) {
	return uses(intIn), nil
}

func (primop *printPrimopT) generate(insts *riscvInstsT, call *cps.CallNodeT) {
	arg0 := call.Inputs[0]
	// 1 = PrintInteger

	insts.addImm(rStack, rStack, -16)
	insts.store(rArg7, rStack, 0)
	insts.store(rArg0, rStack, 8)

	insts.iFormat(1, r0, 0, intRegs[17], 0x13)
	insts.rFormat(0, arg0, r0, 0, intRegs[10], 0x3B)
	insts.iFormat(0, r0, 0, r0, 0x73)

	insts.load(rStack, 0, rArg7)
	insts.load(rStack, 8, rArg0)
	insts.addImm(rStack, rStack, 16)
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
