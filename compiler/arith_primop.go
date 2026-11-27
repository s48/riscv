// Copyright 2026 Richard Kelsey. All rights reserved.
// See file LICENSE for notices and license.

package compiler

import (
	"fmt"
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
	//
	// 'wraps' is true for the operations whose results can be out of
	// range for a narrow type, and so must be re-extended.  The
	// bitwise operations keep extended inputs extended, and unsigned
	// quotients and remainders are no larger than their inputs.
	addPrimop(&binopPrimopT{"+", 0, 0, 0x33, true})
	addPrimop(&binopPrimopT{"-", 0, 0x20, 0x33, true})
	addPrimop(&binopPrimopT{"^", 4, 0, 0x33, false})
	addPrimop(&binopPrimopT{"|", 6, 0, 0x33, false})
	addPrimop(&binopPrimopT{"&", 7, 0, 0x33, false})
	// '/' and '%' are assumed to take unsigned inputs, so they use
	// the U (unsigned) forms, DIVU and REMU; MUL's low 64 bits are
	// the same regardless of signedness, so plain MUL has no separate
	// unsigned form to pick.
	addPrimop(&binopPrimopT{"*", 0, 0x01, 0x33, true})  // MUL
	addPrimop(&binopPrimopT{"/", 5, 0x01, 0x33, false}) // DIVU
	addPrimop(&binopPrimopT{"%", 7, 0x01, 0x33, false}) // REMU
}

type binopPrimopT struct {
	name   string
	funct3 uint32
	funct7 uint32
	op     uint32
	wraps  bool
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
	if primop.wraps {
		insts.extendResult(call)
	}
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
		insts.extendResult(call)
	case token.XOR:
		// xori out, x, -1
		// The complement of a sign-extended value is sign-extended.
		insts.iFormat(-1, call.Inputs[0], 4, call.Outputs[0], 0x13)
		if !isSignedType(call.Outputs[0].Type.(nativeTypeT)) {
			insts.extendResult(call)
		}
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
	addPrimop(&shiftPrimopT{"<<", token.SHL, 0x1, 0x00, true})  // SLL
	addPrimop(&shiftPrimopT{">>", token.SHR, 0x5, 0x00, false}) // SRL
}

type shiftPrimopT struct {
	name       string
	constantOp token.Token
	funct3     uint32
	funct7     uint32
	wraps      bool // see binopPrimopT
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
	if primop.wraps {
		insts.extendResult(call)
	}
}

//----------------------------------------------------------------

func init() {
	addPrimop(&convertIntPrimopT{"int8", 1, true})
	addPrimop(&convertIntPrimopT{"uint8", 1, false})
	addPrimop(&convertIntPrimopT{"int16", 2, true})
	addPrimop(&convertIntPrimopT{"uint16", 2, false})
	addPrimop(&convertIntPrimopT{"int32", 4, true})
	addPrimop(&convertIntPrimopT{"uint32", 4, false})
	addPrimop(&convertIntPrimopT{"int64", 8, true})
	addPrimop(&convertIntPrimopT{"uint64", 8, false})
	addPrimop(&convertIntPrimopT{"int", 8, true})
	addPrimop(&convertIntPrimopT{"uint", 8, false})
}

// Many conversions generate only a move (see generate below).  Removing
// those calls earlier would save the move, but the call's output
// variable has a different type from its input, and that type can
// matter: constant folding depends on it, and so might the code
// generated by a primop that varies with its inputs' types.
//
// An alternative, for another day, is a register use that tells the
// allocator to carry the input's live range through to the output,
// so that the two share a register and no move is needed.
type convertIntPrimopT struct {
	name     string
	numBytes int
	signed   bool
}

func (primop *convertIntPrimopT) Name() string      { return primop.name }
func (primop *convertIntPrimopT) SideEffects() bool { return false }

// This will need to simplify constants.
func (primop *convertIntPrimopT) Simplify(call *cps.CallNodeT) { cps.DefaultSimplify(call) }
func (primop *convertIntPrimopT) RegisterUsage(call *cps.CallNodeT) (regUsesT, regUsesT) {
	return uses(intIn), uses(intOut)
}

// A register holding a value narrower than 64 bits has it sign- or
// zero-extended to 64 bits, according to its type's signedness.  The
// arithmetic primops that can carry out of the low bytes (z + 1 for an
// int8 z, say) re-extend their results; see extendResult.
//
// A conversion is a move if the value is the same in the new type: the
// new type is 64 bits, or it is wider and either the old type is
// unsigned or the new one is signed.  Otherwise, the low bytes are
// extended according to the new type (int8(-1) -> uint16 is 0xFFFF).
func (primop *convertIntPrimopT) generate(insts *riscvInstsT, call *cps.CallNodeT) {
	var argType nativeTypeT
	rawArg := call.Inputs[0]
	switch arg := rawArg.(type) {
	case *cps.ReferenceNodeT:
		argType = arg.Variable.Type.(nativeTypeT)
	case *cps.LiteralNodeT:
		argType = arg.Type.(nativeTypeT)
	default:
		panic(fmt.Sprintf("convertIntPrimopT got an odd argument %T", arg))
	}
	in := call.Inputs[0]
	out := call.Outputs[0]
	argSigned := isSignedType(argType)
	if primop.numBytes == 8 ||
		(argType.sizeBytes() < primop.numBytes && (!argSigned || primop.signed)) {
		if regNumber(in) != regNumber(out) {
			insts.move(in, out)
		}
	} else {
		insts.extend(in, out, primop.numBytes, primop.signed)
	}
}

func isSignedType(typ nativeTypeT) bool {
	switch typ.kind() {
	case int8Type, int16Type, int32Type, int64Type:
		return true
	}
	return false
}

// Sign- or zero-extend the low 'numBytes' of 'source' into 'dest'.
func (insts *riscvInstsT) extend(source any, dest any, numBytes int, signed bool) {
	if signed && numBytes == 4 {
		insts.iFormat(0, source, 0, dest, 0x1B) // addiw dest, source, 0 (sext.w)
		return
	}
	if !signed && numBytes == 1 {
		insts.iFormat(0xFF, source, 7, dest, 0x13) // andi dest, source, 0xFF
		return
	}
	shift := 64 - 8*numBytes
	insts.iFormat(shift, source, 1, dest, 0x13) // slli
	if signed {
		insts.iFormat(0x400|shift, dest, 5, dest, 0x13) // srai
	} else {
		insts.iFormat(shift, dest, 5, dest, 0x13) // srli
	}
}

// Re-extend the result of an operation whose output may have carried
// out of its type's size.  Nothing is needed for 64-bit values.
func (insts *riscvInstsT) extendResult(call *cps.CallNodeT) {
	out := call.Outputs[0]
	typ := out.Type.(nativeTypeT)
	if typ.sizeBytes() < 8 {
		insts.extend(out, out, typ.sizeBytes(), isSignedType(typ))
	}
}
