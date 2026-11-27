// Copyright 2026 Richard Kelsey. All rights reserved.
// See file LICENSE for notices and license.

package compiler

import (
	"go/constant"
	"go/token"

	"github.com/s48/transform/cps"
)

//----------------------------------------------------------------

func init() {
	// These are all 64-bit values, so every one of these uses opcode
	// 0x33 (the plain RV64I/RV64M ops), not 0x3B (the word-sized "W"
	// ops, which truncate to 32 bits and sign-extend back out --
	// wrong for a 64-bit int).
	addPrimop(&binopPrimopT{"+", 0, 0, 0x33})
	addPrimop(&binopPrimopT{"-", 0, 0x20, 0x33})
	addPrimop(&binopPrimopT{"^", 4, 0, 0x33})
	addPrimop(&binopPrimopT{"|", 6, 0, 0x33})
	addPrimop(&binopPrimopT{"&", 7, 0, 0x33})
	// '/' and '%' are assumed to take unsigned inputs, so they use
	// the U (unsigned) forms, DIVU and REMU; MUL's low 64 bits are
	// the same regardless of signedness, so plain MUL has no separate
	// unsigned form to pick.
	addPrimop(&binopPrimopT{"*", 0, 0x01, 0x33}) // MUL
	addPrimop(&binopPrimopT{"/", 5, 0x01, 0x33}) // DIVU
	addPrimop(&binopPrimopT{"%", 7, 0x01, 0x33}) // REMU
}

type binopPrimopT struct {
	name   string
	funct3 uint32
	funct7 uint32
	op     uint32
}

func (primop *binopPrimopT) Name() string                 { return primop.name }
func (primop *binopPrimopT) SideEffects() bool            { return false }
func (primop *binopPrimopT) Simplify(call *cps.CallNodeT) { cps.SimplifyArithBinop(call) }
func (primop *binopPrimopT) RegisterUsage(call *cps.CallNodeT) (regUsesT, regUsesT) {
	return uses(intIn, intIn), uses(intOut)
}

func (primop *binopPrimopT) generate(insts *riscvInstsT, call *cps.CallNodeT) {
	insts.rFormat(
		primop.funct7,
		call.Inputs[1],
		call.Inputs[0],
		primop.funct3,
		call.Outputs[0],
		primop.op)
}

func init() {
	addPrimop(&monopPrimopT{"negate", token.SUB})
	addPrimop(&monopPrimopT{"bitwiseNot", token.XOR})
}

type monopPrimopT struct {
	name  string
	token token.Token
}

func (primop *monopPrimopT) Name() string      { return primop.name }
func (primop *monopPrimopT) SideEffects() bool { return false }
func (primop *monopPrimopT) Simplify(call *cps.CallNodeT) {
	x := call.Inputs[0]
	if !cps.IsLiteralNode(x) {
		cps.DefaultSimplify(call)
		return
	}
	xLit := x.(*cps.LiteralNodeT)
	// For XOR, a nonzero precision gives the unsigned complement
	// (x ^ (1<<prec - 1)), so signed types use 0, which gives -x-1.
	typ := xLit.Type.(nativeTypeT)
	prec := uint(0)
	if !isSignedType(typ) {
		prec = uint(typ.sizeBytes() * 8)
	}
	xLit.Value = constant.UnaryOp(primop.token, xLit.Constant(), prec)
	call.Primop = cps.LookupPrimop("let")
	cps.MarkChanged(call)
}

func (primop *monopPrimopT) RegisterUsage(call *cps.CallNodeT) (regUsesT, regUsesT) {
	return uses(intIn), uses(intOut)
}

func (primop *monopPrimopT) generate(insts *riscvInstsT, call *cps.CallNodeT) {
	switch primop.token {
	case token.SUB:
		// sub out, zero, x
		insts.rFormat(0x20, call.Inputs[0], r0, 0, call.Outputs[0], 0x33)
	case token.XOR:
		// xori out, x, -1
		insts.iFormat(-1, call.Inputs[0], 4, call.Outputs[0], 0x13)
	default:
		panicf("unknown unary operator %s", primop.token)
	}
}

//----------------------------------------------------------------

func init() {
	// Register-register shifts (opcode 0x33): SLL and, since '>>' is
	// assumed to take unsigned input, the logical shift SRL rather
	// than the arithmetic shift SRA.  The emulator already masks the
	// shift amount to the low 6 bits (RV64's shamt width), so there's
	// no need to do that here.
	addPrimop(&shiftPrimopT{"<<", token.SHL, 0x1, 0x00}) // SLL
	addPrimop(&shiftPrimopT{">>", token.SHR, 0x5, 0x00}) // SRL
}

type shiftPrimopT struct {
	name       string
	constantOp token.Token
	funct3     uint32
	funct7     uint32
}

func (primop *shiftPrimopT) Name() string      { return primop.name }
func (primop *shiftPrimopT) SideEffects() bool { return false }
func (primop *shiftPrimopT) Simplify(call *cps.CallNodeT) {
	n := call.Inputs[0]
	shift := call.Inputs[1]
	if cps.IsLiteralNode(n) && cps.IsLiteralNode(shift) {
		nLit := n.(*cps.LiteralNodeT)
		shiftValue := uint(valueAsInt(shift))
		nLit.Value = constant.Shift(nLit.Constant(), primop.constantOp, shiftValue)
		cps.DetachInput(shift)
		cps.RemoveNullInputs(call, 1)
		call.Primop = cps.LookupPrimop("let")
		cps.MarkChanged(call)
		return
	}
	cps.DefaultSimplify(call)
}
func (primop *shiftPrimopT) RegisterUsage(call *cps.CallNodeT) (regUsesT, regUsesT) {
	return uses(intIn, intIn), uses(intOut)
}

func (primop *shiftPrimopT) generate(insts *riscvInstsT, call *cps.CallNodeT) {
	insts.rFormat(
		primop.funct7,
		call.Inputs[1],
		call.Inputs[0],
		primop.funct3,
		call.Outputs[0],
		0x33)
}
