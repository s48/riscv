// Copyright 2026 Richard Kelsey. All rights reserved.
// See file LICENSE for notices and license.

// Most of the code, and this wordy comment, are courtesy of Claude.
//
// Disassembler for the instructions that cpu.go knows how to execute,
// which is RV64IM plus the fence and system instructions.
//
// The output matches objdump, so the two can be diffed: operands are
// separated by commas, pseudo-instructions are used where objdump uses
// them, and immediates are printed in decimal except for lui and auipc,
// shift counts, and branch and jump targets, which are hex.  Anything
// that doesn't decode comes back as a .word directive.
//
// There are three places where we knowingly differ.  Objdump prints the
// floating point CSRs using the aliases from the F extension, which the
// emulator doesn't implement, so "csrrw a0,fflags,a1" comes out here
// rather than "fsflags a0,a1".  A CSR that neither cpu.go nor the table
// at the end of this file names is printed as a number rather than
// objdump's name.  And an empty fence predecessor or successor set is
// printed as "0" rather than objdump's "unknown".
//
// cmd/disasmcheck runs a large synthetic sweep against the real
// toolchain's objdump and checks that every difference falls into one
// of the three categories above.
//
// Compressed (16-bit) instructions are not handled, as the emulator
// doesn't execute them either.

package emulator

import (
	"fmt"
	"strings"
)

// The names GDB uses for the registers.
var gdbRegNames = []string{
	"zero",
	"ra",             // return address
	"sp",             // stack pointer
	"gp",             // global pointer
	"tp",             // thread pointer
	"t0", "t1", "t2", // temps
	"fp", // frame pointer / save 0
	"s1", // callee save 1
	// arguments
	"a0", "a1", "a2", "a3", "a4", "a5", "a6", "a7",
	// more callee saves
	"s2", "s3", "s4", "s5", "s6", "s7", "s8", "s9", "s10", "s11",
	// more temps
	"t3", "t4", "t5", "t6",
}

// Maps address to labels.
type labelMapT map[uint64]string

// Disassemble returns the assembly source for a single instruction.
// The PC is needed because branches and jumps are PC-relative; their
// targets are printed as absolute addresses.
func Disassemble(pc uint64, inst uint32, labels labelMapT) string {
	// The canonical illegal instruction, which compilers emit to mark
	// unreachable code.  It is a CSR write to a read-only CSR.
	if inst == 0xC0001073 {
		return format("unimp")
	}
	switch inst & 0x7F {
	case 0x03:
		return disassembleLoad(inst)
	case 0x0F:
		return disassembleFence(inst)
	case 0x13:
		return disassembleArithmeticImm(inst)
	case 0x17:
		// U format
		return format("auipc", rdName(inst), upperImm(inst))
	case 0x1B:
		return disassembleArithmeticImm64(inst)
	case 0x23:
		return disassembleStore(inst)
	case 0x33:
		if (inst>>25)&0x7F == 1 {
			return disassembleMulDiv(inst)
		} else {
			return disassembleArithmetic(inst)
		}
	case 0x37:
		// U format
		return format("lui", rdName(inst), upperImm(inst))
	case 0x3B:
		if (inst>>25)&0x7F == 1 {
			return disassembleMulDiv64(inst)
		} else {
			return disassembleArithmetic64(inst)
		}
	case 0x63:
		return disassembleBranch(pc, inst, labels)
	case 0x67:
		return disassembleJumpAndLinkReg(inst)
	case 0x6F:
		return disassembleJumpAndLink(pc, inst, labels)
	case 0x73:
		funct3 := (inst >> 12) & 0x7
		if funct3 == 0 {
			return disassembleSystem(inst)
		} else {
			return disassembleCsrReadWrite(inst)
		}
	default:
		return unknown(inst)
	}
}

// DisassembleAt reads the instruction at an address and disassembles it.
func (cpu *CpuT) DisassembleAt(address uint64, labels labelMapT) string {
	return Disassemble(address, cpu.readWord(address), labels)
}

//----------------------------------------------------------------
// Instruction fields.  These match the field extractions in cpu.go.

func rdName(inst uint32) string {
	return gdbRegNames[(inst>>7)&0x1F]
}

func rs1Name(inst uint32) string {
	return gdbRegNames[(inst>>15)&0x1F]
}

func rs2Name(inst uint32) string {
	return gdbRegNames[(inst>>20)&0x1F]
}

func rdField(inst uint32) uint32 {
	return (inst >> 7) & 0x1F
}

func rs1Field(inst uint32) uint32 {
	return (inst >> 15) & 0x1F
}

func rs2Field(inst uint32) uint32 {
	return (inst >> 20) & 0x1F
}

// The immediate of an I format instruction.
func iImm(inst uint32) int64 {
	return signExtend(uint64(inst>>20), 12)
}

// The immediate of a U format instruction, printed the way objdump
// does it, as the unsigned value of the 20-bit field.
func upperImm(inst uint32) string {
	return fmt.Sprintf("0x%x", (inst>>12)&0xFFFFF)
}

//----------------------------------------------------------------
// Output formatting.

func format(name string, operands ...string) string {
	if len(operands) == 0 {
		return name
	}
	return fmt.Sprintf("%-6s %s", name, strings.Join(operands, ","))
}

func decimal(value int64) string {
	return fmt.Sprintf("%d", value)
}

func hex(value uint32) string {
	return fmt.Sprintf("0x%x", value)
}

func hexAddress(value uint64, labels labelMapT) string {
	label := labels[value]
	if label == "" {
		return fmt.Sprintf("0x%x", value)
	} else {
		return label
	}
}

// Printed for anything we can't decode, in the same form as objdump,
// so that the output can still be assembled.
func unknown(inst uint32) string {
	return format(".word", fmt.Sprintf("0x%08x", inst))
}

//----------------------------------------------------------------

var loadNames = []string{"lb", "lh", "lw", "ld", "lbu", "lhu", "lwu"}

func disassembleLoad(inst uint32) string {
	// I format
	// imm:12 rs1:5 funct3:3 rd:5 op:7
	op := (inst >> 12) & 0x7
	if int(op) < len(loadNames) {
		return format(
			loadNames[op],
			rdName(inst),
			fmt.Sprintf("%d(%s)", iImm(inst), rs1Name(inst)))
	}
	return unknown(inst)
}

var storeNames = []string{"sb", "sh", "sw", "sd"}

func disassembleStore(inst uint32) string {
	// S format
	// imm:7 rs2:5 rs1:5 funct3:3 imm:5 op:7
	imm := signExtend(uint64(((inst>>7)&0x1F)|((inst>>25)<<5)), 12)
	op := (inst >> 12) & 0x7
	if int(op) < len(storeNames) {
		return format(
			storeNames[op],
			rs2Name(inst),
			fmt.Sprintf("%d(%s)", imm, rs1Name(inst)))
	}
	return unknown(inst)
}

func disassembleFence(inst uint32) string {
	// Fence uses the I format with the immediate divided into three fields.
	//  fm:4 pred:4 succ:4 rs1:5 funct3:000 rd:5 op:0001111
	funct3 := (inst >> 12) & 0x7
	// The rd and rs1 fields are unused and must be zero.
	if rdField(inst) != 0 || rs1Field(inst) != 0 {
		return unknown(inst)
	}
	if funct3 == 1 {
		if inst>>20 != 0 {
			return unknown(inst)
		}
		return format("fence.i")
	}
	if funct3 != 0 {
		return unknown(inst)
	}
	fm := (inst >> 28) & 0xF
	pred := (inst >> 24) & 0xF
	succ := (inst >> 20) & 0xF
	if fm == 8 && pred == 0x3 && succ == 0x3 {
		return format("fence.tso")
	}
	if fm == 0 && pred == 0x1 && succ == 0x0 {
		return format("pause")
	}

	if fm != 0 {
		return unknown(inst)
	}
	if pred == 0xF && succ == 0xF {
		// A fence of everything against everything is just "fence".
		return format("fence")
	}
	return format("fence", fenceFlags(pred), fenceFlags(succ))
}

// The four bits of a fence predecessor or successor set, high bit first.
func fenceFlags(bits uint32) string {
	flags := ""
	for i, name := range []string{"i", "o", "r", "w"} {
		if bits&(0x8>>i) != 0 {
			flags += name
		}
	}
	if flags == "" {
		return "0"
	}
	return flags
}

func disassembleArithmetic(inst uint32) string {
	// R format
	// funct7:7 rs2:5 rs1:5 funct3:3 rd:5 op:7
	funct3 := (inst >> 12) & 0x7
	funct7 := (inst >> 25) & 0x7F

	var name string
	switch funct7 {
	case 0x00:
		name = []string{"add", "sll", "slt", "sltu", "xor", "srl", "or", "and"}[funct3]
	case 0x20:
		if funct3 == 0x0 {
			name = "sub"
		} else if funct3 == 0x5 {
			name = "sra"
		} else {
			return unknown(inst)
		}
	default:
		return unknown(inst)
	}

	// Pseudo-instructions.
	if rs1Field(inst) == zero {
		switch name {
		case "sub":
			return format("neg", rdName(inst), rs2Name(inst))
		case "sltu":
			return format("snez", rdName(inst), rs2Name(inst))
		}
	}
	if rs2Field(inst) == zero && name == "slt" {
		return format("sltz", rdName(inst), rs1Name(inst))
	}
	if rs1Field(inst) == zero && name == "slt" {
		return format("sgtz", rdName(inst), rs2Name(inst))
	}

	return format(name, rdName(inst), rs1Name(inst), rs2Name(inst))
}

func disassembleArithmetic64(inst uint32) string {
	// R format
	// funct7:7 rs2:5 rs1:5 funct3:3 rd:5 op:7
	funct3 := (inst >> 12) & 0x7
	funct7 := (inst >> 25) & 0x7F

	var name string
	switch funct3 {
	case 0x0:
		if funct7 == 0x00 {
			name = "addw"
		} else if funct7 == 0x20 {
			name = "subw"
		}
	case 0x1:
		if funct7 == 0x00 {
			name = "sllw"
		}
	case 0x5:
		if funct7 == 0x00 {
			name = "srlw"
		} else if funct7 == 0x20 {
			name = "sraw"
		}
	}
	if name == "" {
		return unknown(inst)
	}
	if name == "subw" && rs1Field(inst) == zero {
		return format("negw", rdName(inst), rs2Name(inst))
	}
	return format(name, rdName(inst), rs1Name(inst), rs2Name(inst))
}

func disassembleMulDiv(inst uint32) string {
	// R format
	// funct7:7 rs2:5 rs1:5 funct3:3 rd:5 op:7
	name := []string{"mul", "mulh", "mulhsu", "mulhu", "div", "divu", "rem", "remu"}[(inst>>12)&0x7]
	return format(name, rdName(inst), rs1Name(inst), rs2Name(inst))
}

var mulDivNames = []string{"mulw", "", "", "", "divw", "divuw", "remw", "remuw"}

func disassembleMulDiv64(inst uint32) string {
	// R format
	// funct7:7 rs2:5 rs1:5 funct3:3 rd:5 op:7
	op := int((inst >> 12) & 0x7)
	if len(mulDivNames) <= op || mulDivNames[op] == "" {
		return unknown(inst)
	}
	return format(mulDivNames[op], rdName(inst), rs1Name(inst), rs2Name(inst))
}

func disassembleArithmeticImm(inst uint32) string {
	// I format
	// imm:12 rs1:5 funct3:3 rd:5 op:7
	funct3 := (inst >> 12) & 0x7
	imm := iImm(inst)

	// The shifts use the low six bits of the immediate as the shift
	// count and the high six to pick the operation.
	shiftCount := hex((inst >> 20) & 0x3F)
	funct6 := (inst >> 26) & 0x3F

	switch funct3 {
	case 0x0: // ADDI
		if rdField(inst) == zero && rs1Field(inst) == zero && imm == 0 {
			return format("nop")
		}
		if rs1Field(inst) == zero {
			return format("li", rdName(inst), decimal(imm))
		}
		if imm == 0 {
			return format("mv", rdName(inst), rs1Name(inst))
		}
		return format("addi", rdName(inst), rs1Name(inst), decimal(imm))
	case 0x1: // SLLI
		if funct6 != 0x00 {
			return unknown(inst)
		}
		return format("slli", rdName(inst), rs1Name(inst), shiftCount)
	case 0x2: // SLTI
		return format("slti", rdName(inst), rs1Name(inst), decimal(imm))
	case 0x3: // SLTIU
		if imm == 1 {
			return format("seqz", rdName(inst), rs1Name(inst))
		}
		return format("sltiu", rdName(inst), rs1Name(inst), decimal(imm))
	case 0x4: // XORI
		if imm == -1 {
			return format("not", rdName(inst), rs1Name(inst))
		}
		return format("xori", rdName(inst), rs1Name(inst), decimal(imm))
	case 0x5: // SRLI / SRAI
		if funct6 == 0x00 {
			return format("srli", rdName(inst), rs1Name(inst), shiftCount)
		} else if funct6 == 0x10 {
			return format("srai", rdName(inst), rs1Name(inst), shiftCount)
		} else {
			return unknown(inst)
		}
	case 0x6: // ORI
		return format("ori", rdName(inst), rs1Name(inst), decimal(imm))
	case 0x7: // ANDI
		return format("andi", rdName(inst), rs1Name(inst), decimal(imm))
	}
	return unknown(inst)
}

func disassembleArithmeticImm64(inst uint32) string {
	// I format
	// imm:12 rs1:5 funct3:3 rd:5 op:7
	funct3 := (inst >> 12) & 0x7
	funct7 := (inst >> 25) & 0x7F
	imm := iImm(inst)
	shiftCount := hex((inst >> 20) & 0x1F)

	switch funct3 {
	case 0x0: // ADDIW
		if imm == 0 {
			return format("sext.w", rdName(inst), rs1Name(inst))
		}
		return format("addiw", rdName(inst), rs1Name(inst), decimal(imm))
	case 0x1: // SLLIW
		if funct7 != 0x00 {
			return unknown(inst)
		}
		return format("slliw", rdName(inst), rs1Name(inst), shiftCount)
	case 0x5: // SRLIW / SRAIW
		if funct7 == 0x00 {
			return format("srliw", rdName(inst), rs1Name(inst), shiftCount)
		} else if funct7 == 0x20 {
			return format("sraiw", rdName(inst), rs1Name(inst), shiftCount)
		}
	}
	return unknown(inst)
}

var branchNames = []string{"beq", "bne", "", "", "blt", "bge", "bltu", "bgeu"}

func disassembleBranch(pc uint64, inst uint32, labels labelMapT) string {
	// B format
	// imm7:7 rs2:5 rs1:5 funct3:3 imm:5 op:7
	// The immediate bits are a bit scrambled.
	op := int((inst >> 12) & 0x7)
	if len(branchNames) <= op || branchNames[op] == "" {
		return unknown(inst)
	}
	name := branchNames[op]
	imm := signExtend(uint64(
		((inst>>7)&0x1E)|
			((inst>>20)&0x7E0)|
			((inst<<4)&0x800)|
			((inst>>19)&0x1000)), 13)
	target := hexAddress(pc+uint64(imm), labels)

	// Comparisons against zero have their own names.
	if rs1Field(inst) == zero && name == "bge" {
		return format("blez", rs2Name(inst), target)
	}
	if rs2Field(inst) == zero {
		switch name {
		case "beq", "bne", "blt", "bge":
			return format(name+"z", rs1Name(inst), target)
		}
	}
	if rs1Field(inst) == zero && name == "blt" {
		return format("bgtz", rs2Name(inst), target)
	}
	return format(name, rs1Name(inst), rs2Name(inst), target)
}

func disassembleJumpAndLinkReg(inst uint32) string {
	// I format
	// imm:12 rs1:5 funct3:3 rd:5 op:7
	if (inst>>12)&0x7 != 0 {
		return unknown(inst)
	}
	imm := iImm(inst)
	offset := fmt.Sprintf("%d(%s)", imm, rs1Name(inst))
	if rdField(inst) == zero {
		if imm == 0 {
			if rs1Field(inst) == returnAddress {
				return format("ret")
			}
			return format("jr", rs1Name(inst))
		}
		return format("jr", offset)
	}
	if rdField(inst) == returnAddress {
		if imm == 0 {
			return format("jalr", rs1Name(inst))
		}
		return format("jalr", offset)
	}
	if imm == 0 {
		return format("jalr", rdName(inst), rs1Name(inst))
	}
	return format("jalr", rdName(inst), offset)
}

func disassembleJumpAndLink(pc uint64, inst uint32, labels labelMapT) string {
	// J format
	// imm:20 rd:5 op:7
	// The immediate bits are quite scrambled.
	imm := signExtend(uint64(
		((inst>>20)&0x7FE)|
			((inst>>9)&0x800)|
			(inst&0xFF000)|
			((inst>>11)&0x100000)), 21)
	target := hexAddress(pc+uint64(imm), labels)
	if rdField(inst) == zero {
		return format("j", target)
	}
	if rdField(inst) == returnAddress {
		return format("jal", target)
	}
	return format("jal", rdName(inst), target)
}

var syscalls = map[uint]string{
	0x000: "ecall",
	0x001: "ebreak",
	0x002: "uret",
	0x102: "sret",
	0x302: "mret",
	0x105: "wfi",
}

func disassembleSystem(inst uint32) string {
	// I format
	// imm:12 rs1:5 funct3:3 rd:5 op:7
	if rdField(inst) != 0 {
		return unknown(inst)
	}
	imm := uint(inst >> 20)
	if (imm>>5) == 0x09 && rdField(inst) == 0 {
		// sfence.vma rs1, rs2
		return format("sfence.vma", rs1Name(inst), rs2Name(inst))
	}
	if rs1Field(inst) != 0 {
		return unknown(inst)
	}
	name, found := syscalls[imm]
	if found {
		return format(name)
	}
	return unknown(inst)
}

func disassembleCsrReadWrite(inst uint32) string {
	// I format
	// imm:12 rs1:5 funct3:3 rd:5 op:7
	funct3 := (inst >> 12) & 0x7
	number := uint64(inst >> 20)
	name := csrName(number)

	// The immediate forms hold a five bit unsigned value in the rs1 field.
	immediate := funct3 >= 5
	var source string
	if immediate {
		source = decimal(int64(rs1Field(inst)))
	} else {
		source = rs1Name(inst)
	}

	op := []string{"", "w", "s", "c"}[funct3&0x3]
	if op == "" {
		return unknown(inst)
	}

	// Pseudo-instructions: setting no bits is just a read, and any
	// of the three that discards the old value is just a write.
	if op == "s" && !immediate && rs1Field(inst) == 0 {
		return format("csrr", rdName(inst), name)
	}
	if rdField(inst) == zero {
		suffix := ""
		if immediate {
			suffix = "i"
		}
		return format("csr"+op+suffix, name, source)
	}

	full := "csrr" + op
	if immediate {
		full += "i"
	}
	return format(full, rdName(inst), name, source)
}

// The name of a CSR, or its number if we don't have a name for it.
// The CSRs the emulator implements are named by cpu.go; the rest are
// in the table below so that the disassembly of code we can't run is
// still readable.
func csrName(number uint64) string {
	csr := csrs[number]
	if csr != nil {
		return csr.name
	}
	name, found := csrNames[number]
	if found {
		return name
	}
	return fmt.Sprintf("0x%x", number)
}

var csrNames = map[uint64]string{}

func init() {
	add := func(number uint64, name string) {
		csrNames[number] = name
	}
	// A family of CSRs with consecutive numbers, named by index,
	// as in pmpaddr0 ... pmpaddr63.  The suffix is "h" for the
	// registers holding the high halves of the RV32 counters.
	addFamily := func(number uint64, prefix string, first int, count int, suffix string) {
		for i := 0; i < count; i++ {
			add(number+uint64(i), fmt.Sprintf("%s%d%s", prefix, first+i, suffix))
		}
	}

	// Unprivileged floating point and counters.
	add(0x001, "fflags")
	add(0x002, "frm")
	add(0x003, "fcsr")
	add(0xC00, "cycle")
	add(0xC01, "time")
	add(0xC02, "instret")
	addFamily(0xC03, "hpmcounter", 3, 29, "")
	add(0xC80, "cycleh")
	add(0xC81, "timeh")
	add(0xC82, "instreth")
	addFamily(0xC83, "hpmcounter", 3, 29, "h") // the high halves, for RV32

	// Supervisor.
	add(0x100, "sstatus")
	add(0x104, "sie")
	add(0x105, "stvec")
	add(0x106, "scounteren")
	add(0x10A, "senvcfg")
	add(0x140, "sscratch")
	add(0x141, "sepc")
	add(0x142, "scause")
	add(0x143, "stval")
	add(0x144, "sip")
	add(0x180, "satp")

	// Machine information and trap setup.
	add(0xF11, "mvendorid")
	add(0xF12, "marchid")
	add(0xF13, "mimpid")
	add(0xF14, "mhartid")
	add(0xF15, "mconfigptr")
	add(0x300, "mstatus")
	add(0x301, "misa")
	add(0x302, "medeleg")
	add(0x303, "mideleg")
	add(0x304, "mie")
	add(0x305, "mtvec")
	add(0x306, "mcounteren")
	add(0x310, "mstatush")
	add(0x30A, "menvcfg")
	add(0x31A, "menvcfgh")
	add(0x747, "mseccfg")
	add(0x757, "mseccfgh")

	// Machine trap handling.
	add(0x340, "mscratch")
	add(0x341, "mepc")
	add(0x342, "mcause")
	add(0x343, "mtval")
	add(0x344, "mip")
	add(0x34A, "mtinst")
	add(0x34B, "mtval2")

	// Physical memory protection.
	addFamily(0x3A0, "pmpcfg", 0, 16, "")
	addFamily(0x3B0, "pmpaddr", 0, 64, "")

	// Machine counters.
	add(0xB00, "mcycle")
	add(0xB02, "minstret")
	addFamily(0xB03, "mhpmcounter", 3, 29, "")
	add(0xB80, "mcycleh")
	add(0xB82, "minstreth")
	addFamily(0xB83, "mhpmcounter", 3, 29, "h") // the high halves, for RV32
	add(0x320, "mcountinhibit")
	addFamily(0x323, "mhpmevent", 3, 29, "")

	// Debug and trace.
	add(0x7A0, "tselect")
	add(0x7A1, "tdata1")
	add(0x7A2, "tdata2")
	add(0x7A3, "tdata3")
	add(0x7A5, "tcontrol")
	add(0x7B0, "dcsr")
	add(0x7B1, "dpc")
	add(0x7B2, "dscratch0")
	add(0x7B3, "dscratch1")

	// Hypervisor and virtual supervisor.
	add(0x200, "vsstatus")
	add(0x204, "vsie")
	add(0x205, "vstvec")
	add(0x240, "vsscratch")
	add(0x241, "vsepc")
	add(0x242, "vscause")
	add(0x243, "vstval")
	add(0x244, "vsip")
	add(0x280, "vsatp")
	add(0x600, "hstatus")
	add(0x602, "hedeleg")
	add(0x603, "hideleg")
	add(0x604, "hie")
	add(0x605, "htimedelta")
	add(0x606, "hcounteren")
	add(0x607, "hgeie")
	add(0x60A, "henvcfg")
	add(0x615, "htimedeltah")
	add(0x61A, "henvcfgh")
	add(0x643, "htval")
	add(0x644, "hip")
	add(0x645, "hvip")
	add(0x64A, "htinst")
	add(0x680, "hgatp")
	add(0xE12, "hgeip")

	// Resumable non-maskable interrupts.
	add(0x740, "mnscratch")
	add(0x741, "mnepc")
	add(0x742, "mncause")
	add(0x744, "mnstatus")
}
