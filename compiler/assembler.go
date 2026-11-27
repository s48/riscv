// Copyright 2026 Richard Kelsey. All rights reserved.
// See file LICENSE for notices and license.

package compiler

import (
	"fmt"
	"math/bits"

	"github.com/s48/transform/cps"
)

type riscvInstsT struct {
	instructions []uint32
	frame        *frameT
	callTo       *cps.CallNodeT // procedure called by the last instruction
	usesHeap     bool           // set by makeCell, so the heap prelude is needed
}

// funct7:7 rs2:5 rs1:5 funct3:3 rd:5 op:7
func (insts *riscvInstsT) rFormat(
	funct7 uint32,
	rs2 any,
	rs1 any,
	funct3 uint32,
	rd any,
	op uint32,
) {
	inst := makeInst([]int{7, 5, 5, 3, 5, 7},
		funct7, regNumber(rs2), regNumber(rs1), funct3, regNumber(rd), op)
	cps.Push(&insts.instructions, inst)
}

// imm7:7 rs2:5 rs1:5 funct3:3 imm5:5 op:7
// imm7 11:5   imm5 4:0
func (insts *riscvInstsT) sFormat(imm12s int, rs2 any, rs1 any, funct3 uint32, op uint32) {
	if 12 < signedLen(imm12s) {
		panic("sFormat imm12 out of range")
	}
	imm7 := uint32((imm12s >> 5) & 0x7F)
	imm5 := uint32(imm12s & 0x1F)
	inst := makeInst([]int{7, 5, 5, 3, 5, 7},
		imm7, regNumber(rs2), regNumber(rs1), funct3, imm5, op)
	cps.Push(&insts.instructions, inst)
}

// imm:12 rs1:5 funct3:3 rd:5 op:7
func (insts *riscvInstsT) iFormat(imm12s int, rs1 any, funct3 uint32, rd any, op uint32) {
	if 12 < signedLen(imm12s) {
		panic("iFormat imm12 out of range")
	}
	inst := makeInst([]int{12, 5, 3, 5, 7},
		uint32(imm12s&0xFFF), regNumber(rs1), funct3, regNumber(rd), op)
	cps.Push(&insts.instructions, inst)
}

// imm7:7 rs2:5 rs1:5 funct3:3 imm5:5 op:7
// imm7[12|10:5]           imm5[4:1|11]
func (insts *riscvInstsT) bFormat(imm13s int, rs2 any, rs1 any, funct3 uint32, op uint32) {
	if 13 < signedLen(imm13s) {
		panic("bFormat imm13 out of range")
	}
	if imm13s&1 != 0 {
		panic("bFormat imm13 is odd")
	}
	imm13 := uint32(imm13s & 0x1FFE)
	imm7 := ((imm13 >> 12) << 6) | ((imm13 >> 5) & 0x3F)
	imm5 := (imm13 & 0x1E) | ((imm13 >> 11) & 1)
	inst := makeInst([]int{7, 5, 5, 3, 5, 7},
		imm7, regNumber(rs2), regNumber(rs1), funct3, imm5, op)
	cps.Push(&insts.instructions, inst)
}

// imm:20 rd:5 op:7
// imm:20 is bits 31:12 of the result
func (insts *riscvInstsT) uFormat(imm20, rd, op uint32) {
	inst := makeInst([]int{20, 5, 7}, imm20, rd, op)
	cps.Push(&insts.instructions, inst)
}

// imm21 bits are scrambled (and the low bit is dropped)
//
//	imm[20|10:1|11|19:12]
func (insts *riscvInstsT) jFormat(imm21s int, rd any, op uint32) {
	if 21 < signedLen(imm21s) {
		panic("bFormat imm20 out of range")
	}
	if imm21s&1 != 0 {
		panic("bFormat imm20 is odd")
	}
	imm20 := uint32(imm21s&0x1FFFFE) >> 1
	imm20 = (imm20 & 0x80000) |
		((imm20 & 0x003FF) << 9) |
		((imm20 & 0x00400) >> 2) |
		((imm20 & 0x7F800) >> 11)
	inst := makeInst([]int{20, 5, 7}, imm20, regNumber(rd), op)
	cps.Push(&insts.instructions, inst)
}

func makeInst(widths []int, values ...uint32) uint32 {
	if len(widths) != len(values) {
		panic("widths and values don't match")
	}
	inst := uint32(0)
	totalWidth := 0
	for i, width := range widths {
		value := values[i]
		if width < bits.Len32(value) {
			panic("value too large")
		}
		inst = (inst << width) | uint32(value)
		totalWidth += width
	}
	if totalWidth != 32 {
		panic("instruction not 32 bits")
	}
	return inst
}

//----------------------------------------------------------------

func (insts *riscvInstsT) move(source any, dest any) {
	insts.iFormat(0, source, 0, dest, 0x13)
}

func (insts *riscvInstsT) addImm(source any, dest any, imm int) {
	insts.iFormat(imm, source, 0, dest, 0x13)
}

func (insts *riscvInstsT) store(source any, address any, offset int) {
	// 3 = 64-bit double word (2 would be a 32-bit word)
	insts.sFormat(offset, source, address, 3, 0x23)
}

func (insts *riscvInstsT) load(address any, offset int, dest any) {
	// 3 = 64-bit double word (2 would be a sign-extended 32-bit word)
	insts.iFormat(offset, address, 3, dest, 0x03)
}

// Load 'numBytes' (1, 2, 4, or 8) from 'offset(address)', sign- or
// zero-extending them.
func (insts *riscvInstsT) loadSized(address any, offset int, dest any, numBytes int, signed bool) {
	funct3 := uint32(bits.TrailingZeros(uint(numBytes))) // lb lh lw ld
	if !signed && numBytes < 8 {
		funct3 |= 4 // lbu lhu lwu
	}
	insts.iFormat(offset, address, funct3, dest, 0x03)
}

// Store the low 'numBytes' (1, 2, 4, or 8) of 'source' at 'offset(address)'.
func (insts *riscvInstsT) storeSized(source any, address any, offset int, numBytes int) {
	funct3 := uint32(bits.TrailingZeros(uint(numBytes))) // sb sh sw sd
	insts.sFormat(offset, source, address, funct3, 0x23)
}

// Branch to 'offset' bytes from this instruction if 'rs1 <funct3> rs2'.
// funct3 is 0 beq, 1 bne, 4 blt, 5 bge, 6 bltu, 7 bgeu.
func (insts *riscvInstsT) branch(funct3 uint32, rs1 any, rs2 any, offset int) {
	insts.bFormat(offset, rs2, rs1, funct3, 0x63)
}

// rd = imm20 << 12, sign-extended from 32 bits
func (insts *riscvInstsT) lui(imm20 uint32, rd any) {
	insts.uFormat(imm20, regNumber(rd), 0x37)
}

// rd = pc + (imm20 << 12), sign-extended from 32 bits
func (insts *riscvInstsT) auipc(imm20 uint32, rd any) {
	insts.uFormat(imm20, regNumber(rd), 0x17)
}

//----------------------------------------------------------------

func regNumber(rawValue any) uint32 {
	switch value := rawValue.(type) {
	case *registerT:
		return uint32(value.number)
	case *cps.VariableT:
		switch reg := value.Register().(type) {
		case *registerT:
			return uint32(reg.number)
		default:
			panic(fmt.Sprintf("Variable %s has no register '%v'", value, reg))
		}
	case *cps.ReferenceNodeT:
		return uint32(value.Variable.ReferenceRegister(value).(*registerT).number)
	default:
		panic(fmt.Sprintf("Cannot coerce to register '%v' (%T)", rawValue, rawValue))
	}
}

// Returns the minimum field width needed to store 'n' as a signed integer.
func signedLen(n int) int {
	if n < 0 {
		n = -(n + 1)
	}
	// +1 for the sign bit at the front
	return 1 + bits.Len(uint(n))
}
