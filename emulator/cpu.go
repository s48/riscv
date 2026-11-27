// Copyright 2026 Richard Kelsey. All rights reserved.
// See file LICENSE for notices and license.

package emulator

import (
	"encoding/binary"
	"fmt"
	"math/big"
)

// The test programs put the code here, for some reason.
const MemoryStart = 0x80000000

const (
	// standard register assignments
	zero = iota
	returnAddress
	stackPointer
	globalPointer
	threadPointer
	temp0
	temp1
	temp2
	saved0
	saved1
	arg0
	arg1
	arg2
	arg3
	arg4
	arg5
	arg6
	arg7
	saved2
	saved3
	saved4
	saved5
	saved6
	saved7
	saved8
	saved9
	saved10
	saved11
	temp3
	temp4
	temp5
	temp6
	framePointer = saved0
)

type CpuT struct {
	Pc          uint64
	Registers   [32]uint64
	csrValues   map[*csrT]uint64
	memory      []*[65536]byte
	breakpoints map[uint64]struct{}
	Status      CpuStatusT
	Output      []uint64
	ExitStatus  uint8
}

func NewCpu() *CpuT {
	cpu := &CpuT{}
	cpu.memory = make([]*[65536]byte, 64)
	cpu.memory[0] = new([65536]byte)
	cpu.memory[2] = new([65536]byte) // stack
	cpu.memory[5] = new([65536]byte) // thread object and heap
	cpu.csrValues = map[*csrT]uint64{}
	cpu.breakpoints = map[uint64]struct{}{}

	cpu.Registers[returnAddress] = MemoryStart + 0x20000
	cpu.Registers[stackPointer] = MemoryStart + 0x30000
	cpu.Registers[globalPointer] = MemoryStart + 0x40000
	cpu.Registers[threadPointer] = MemoryStart + 0x50000
	cpu.Registers[framePointer] = MemoryStart + 0x30000 // x80000

	return cpu
}

type CpuStatusT int

const (
	cpuStopped CpuStatusT = iota
	cpuRunning
	cpuBreakpoint
	cpuExited
)

func (cpu *CpuT) addBreakpoint(addr uint64) {
	cpu.breakpoints[addr] = struct{}{}
}

func (cpu *CpuT) removeBreakpoint(addr uint64) {
	delete(cpu.breakpoints, addr)
}

func (cpu *CpuT) runOneInstruction() {
	if cpu.Status == cpuExited {
		return
	}
	cpu.Status = cpuRunning
	cpu.execute()
	if cpu.Status == cpuRunning {
		cpu.Status = cpuStopped
	}
}

func (cpu *CpuT) Run() {
	if cpu.Status == cpuExited {
		return
	}
	cpu.Status = cpuRunning
	for cpu.Status == cpuRunning {
		cpu.execute()
	}
}

func (cpu *CpuT) execute() {
	inst := cpu.readWord(cpu.Pc)
	//fmt.Printf("pc %08X ra %08X sp %08X \n", cpu.Pc, cpu.Registers[1], cpu.Registers[2])
	//fmt.Printf("pc %08X %032b\n", cpu.Pc, inst)
	if cpu.Registers[0] != 0 {
		panic("start: r0 is not 0\n")
	}
	//fmt.Printf("%X: %s\n", cpu.Pc, Disassemble(cpu.Pc, inst, nil))
	switch inst & 0x7F {
	case 0x03:
		cpu.executeLoad(inst)
		cpu.Pc += 4
	case 0x0F:
		// Fence uses the I format with the immediate divided into three fields.
		//  fm:4 pred:4 succ:4 xs1:5 funct3:000 xd:5 op:0001111
		//		if funct3 == 0 {
		//			// fence
		//		} else {
		//			missing[op] += 1
		// }
		// panic("oops - fence")
		cpu.Pc += 4
	case 0x13:
		cpu.executeArithmeticImm(inst)
		cpu.Pc += 4
	case 0x17:
		// U format
		cpu.setRd(inst, cpu.Pc+uint64(signExtend(uint64(inst&0xFFFFF000), 32)))
		cpu.Pc += 4
	case 0x1B:
		cpu.executeArithmeticImm64(inst)
		cpu.Pc += 4
	case 0x23:
		cpu.executeStore(inst)
		cpu.Pc += 4
	case 0x33:
		if (inst>>25)&0x7F == 1 {
			cpu.executeMulDiv(inst)
		} else {
			cpu.executeArithmetic(inst)
		}
		cpu.Pc += 4
	case 0x37:
		// U format
		cpu.setRd(inst, uint64(signExtend(uint64(inst&0xFFFFF000), 32)))
		cpu.Pc += 4
	case 0x3B:
		if (inst>>25)&0x7F == 1 {
			cpu.executeMulDiv64(inst)
		} else {
			cpu.executeArithmetic64(inst)
		}
		cpu.Pc += 4
	case 0x63:
		cpu.Pc += uint64(cpu.executeBranch(inst))
	case 0x67:
		// jump and link reg
		// I format
		rs1 := cpu.Registers[(inst>>15)&0x1F]
		imm := signExtend(uint64(inst>>20), 12)
		cpu.setRd(inst, cpu.Pc+4)
		cpu.Pc = (rs1 + uint64(imm)) & ^uint64(1)
	case 0x6F:
		// jump and link
		// J format
		// imm:20 rd:5 op:7
		// The immediate bits are quite scrambled.
		cpu.setRd(inst, cpu.Pc+4)
		imm :=
			((inst >> 20) & 0x7FE) |
				((inst >> 9) & 0x800) |
				(inst & 0xFF000) |
				((inst >> 11) & 0x100000)
		offset := uint64(signExtend(uint64(imm), 21))
		cpu.Pc += offset & ^uint64(1)
	case 0x73:
		funct3 := (inst >> 12) & 0x7
		if funct3 == 0 {
			cpu.Pc = cpu.executeSystemCall(inst)
		} else {
			cpu.executeCsrReadWrite(inst)
			cpu.Pc += 4
		}
	default:
		panic(fmt.Sprintf("No decoder for PC %X Instruction %08X (%32b)",
			cpu.Pc, inst, inst))
	}
	if cpu.Registers[0] != 0 {
		panic("end: r0 is not 0\n")
	}
	_, found := cpu.breakpoints[cpu.Pc]
	if found {
		cpu.Status = cpuBreakpoint
	}
}

func (cpu *CpuT) setRd(inst uint32, value uint64) {
	reg := (inst >> 7) & 0x1F
	if reg != 0 {
		cpu.Registers[reg] = value
	}
}

func (cpu *CpuT) executeArithmetic(inst uint32) {
	// R format
	// funct7:7 rs2:5 rs1:5 funct3:3 rd:5 op:7
	funct3 := (inst >> 12) & 0x7
	funct7 := (inst >> 25) & 0x7F

	rs1 := cpu.Registers[(inst>>15)&0x1F]
	rs2 := cpu.Registers[(inst>>20)&0x1F]
	signedRs1 := int64(rs1)
	signedRs2 := int64(rs2)

	var result uint64

	switch funct3 {
	case 0x0:
		if funct7 == 0x00 {
			result = uint64(signedRs1 + signedRs2) // ADD
		} else if funct7 == 0x20 {
			result = uint64(signedRs1 - signedRs2) // SUB
		} else {
			panic("oops")
		}
	case 0x1: // SLL
		result = rs1 << (rs2 & 0x3F)
	case 0x2: // SLT
		if signedRs1 < signedRs2 {
			result = 1
		} else {
			result = 0
		}
	case 0x3: // SLTU
		if rs1 < rs2 {
			result = 1
		} else {
			result = 0
		}
	case 0x4: // XOR
		result = rs1 ^ rs2
	case 0x5:
		if funct7 == 0x00 {
			result = rs1 >> (rs2 & 0x3F) // SRL
		} else if funct7 == 0x20 {
			result = uint64(signedRs1 >> (rs2 & 0x3F)) // SRA
		} else {
			panic("oops")
		}
	case 0x6: // OR
		result = rs1 | rs2
	case 0x7: // AND
		result = rs1 & rs2
	}
	cpu.setRd(inst, result)
}

func (cpu *CpuT) executeArithmetic64(inst uint32) {
	// R format
	// funct7:7 rs2:5 rs1:5 funct3:3 rd:5 op:7
	funct3 := (inst >> 12) & 0x7
	funct7 := (inst >> 25) & 0x7F

	rs1 := cpu.Registers[(inst>>15)&0x1F]
	rs2 := cpu.Registers[(inst>>20)&0x1F]
	signedRs1 := int64(rs1)
	signedRs2 := int64(rs2)

	var result uint64

	switch funct3 {
	case 0x0:
		if funct7 == 0x00 {
			result = uint64(signExtend(uint64(signedRs1+signedRs2), 32))
		} else if funct7 == 0x20 {
			result = uint64(signExtend(uint64(signedRs1-signedRs2), 32))
		} else {
			panic("oops")
		}
	case 0x1: // SLL
		result = uint64(signExtend(rs1<<(rs2&0x1F), 32))
	case 0x4: // XOR
		result = rs1 ^ rs2
	case 0x5:
		if funct7 == 0x00 {
			result = uint64(signExtend((rs1&0xFFFFFFFF)>>(rs2&0x1F), 32))
		} else if funct7 == 0x20 {
			result = uint64(signExtend(uint64(int32(rs1&0xFFFFFFFF)>>(rs2&0x1F)), 32))
		} else {
			panic("oops")
		}
	case 0x6: // OR
		result = rs1 | rs2
	case 0x7: // AND
		result = rs1 & rs2
	default:
		panic("oops")
	}
	cpu.setRd(inst, result)
}

func (cpu *CpuT) executeMulDiv(inst uint32) {
	// R format
	// funct7:7 rs2:5 rs1:5 funct3:3 rd:5 op:7
	funct3 := (inst >> 12) & 0x7

	rs1 := cpu.Registers[(inst>>15)&0x1F]
	rs2 := cpu.Registers[(inst>>20)&0x1F]
	signedRs1 := int64(rs1)
	signedRs2 := int64(rs2)

	var result uint64

	switch funct3 {
	case 0x0: // MUL
		result = uint64((signedRs1 * signedRs2))
	case 0x1: // MULH
		temp := big.NewInt(0).SetInt64(signedRs1)
		temp.Mul(temp, big.NewInt(0).SetInt64(signedRs2))
		result = temp.Rsh(temp, 64).Uint64()
	case 0x2: // MULHSU
		if signedRs1 < 0 {
			// Put 64 1-bits at the front of rs1, then multiply.
			temp := big.NewInt(0).SetUint64(^uint64(0))
			temp.Lsh(temp, 64).Or(temp, big.NewInt(0).SetUint64(rs1))
			temp.Mul(temp, big.NewInt(0).SetUint64(rs2))
			result = temp.Rsh(temp, 64).Uint64()
		} else {
			temp := big.NewInt(0).SetUint64(rs1)
			temp.Mul(temp, big.NewInt(0).SetUint64(rs2))
			result = temp.Rsh(temp, 64).Uint64()
		}
	case 0x3: // MULHU
		temp := big.NewInt(0).SetUint64(rs1)
		temp.Mul(temp, big.NewInt(0).SetUint64(rs2))
		result = temp.Rsh(temp, 64).Uint64()
	case 0x4: // DIV
		if rs2 == 0 {
			result = ^uint64(0)
		} else {
			result = uint64(signedRs1 / signedRs2)
		}
	case 0x5: // DIVU
		if rs2 == 0 {
			result = ^uint64(0)
		} else {
			result = rs1 / rs2
		}
	case 0x6: // REM
		if rs2 == 0 {
			result = rs1
		} else {
			result = uint64(signedRs1 % signedRs2)
		}
	case 0x7: // REMU
		if rs2 == 0 {
			result = rs1
		} else {
			result = rs1 % rs2
		}
	}
	cpu.setRd(inst, result)
}

func (cpu *CpuT) executeMulDiv64(inst uint32) {
	// R format
	// funct7:7 rs2:5 rs1:5 funct3:3 rd:5 op:7
	funct3 := (inst >> 12) & 0x7

	rs1 := uint32(cpu.Registers[(inst>>15)&0x1F])
	rs2 := uint32(cpu.Registers[(inst>>20)&0x1F])
	signedRs1 := int32(rs1)
	signedRs2 := int32(rs2)

	var result uint32

	switch funct3 {
	case 0x0: // MULW
		result = rs1 * rs2
	case 0x4: // DIVW
		if rs2 == 0 {
			result = ^uint32(0)
		} else {
			result = uint32(signedRs1 / signedRs2)
		}
	case 0x5: // DIVUW
		if rs2 == 0 {
			result = ^uint32(0)
		} else {
			result = rs1 / rs2
		}
	case 0x6: // REMW
		if rs2 == 0 {
			result = rs1
		} else {
			result = uint32(signedRs1 % signedRs2)
		}
	case 0x7: // REMUW
		if rs2 == 0 {
			result = rs1
		} else {
			result = rs1 % rs2
		}
	}
	cpu.setRd(inst, uint64(signExtend(uint64(result), 32)))
}

func (cpu *CpuT) executeArithmeticImm(inst uint32) {
	// I format
	// imm:12 rs1:5 funct3:3 rd:5 op:7

	funct3 := (inst >> 12) & 0x7
	funct7 := (inst >> 25) & 0x7F

	rs1 := cpu.Registers[(inst>>15)&0x1F]
	signedRs1 := int64(rs1)

	imm := uint64(inst >> 20)
	signedImm := signExtend(imm, 12)
	uimm := imm & 0x3F

	var result uint64

	switch funct3 {
	case 0x0: // ADDI
		result = uint64(signedRs1 + signedImm)
	case 0x1:
		result = rs1 << uimm
	case 0x2: // SLTI
		if signedRs1 < signedImm {
			result = 1
		} else {
			result = 0
		}
	case 0x3: // SLTIU
		if rs1 < uint64(signedImm) {
			result = 1
		} else {
			result = 0
		}
	case 0x4: // XORI
		result = rs1 ^ uint64(signedImm)
	case 0x5:
		if funct7 == 0x00 {
			result = rs1 >> uimm
		} else if funct7 == 0x20 {
			result = uint64(signedRs1 >> uimm)
		} else {
			panic("oops")
		}
	case 0x6: // ORI
		result = rs1 | uint64(signedImm)
	case 0x7: // ANDI
		result = rs1 & uint64(signedImm)
	}
	cpu.setRd(inst, result)
}

func (cpu *CpuT) executeArithmeticImm64(inst uint32) {
	// I format
	// imm:12 rs1:5 funct3:3 rd:5 op:7

	funct3 := (inst >> 12) & 0x7
	funct7 := (inst >> 25) & 0x7F

	rs1 := cpu.Registers[(inst>>15)&0x1F]
	signedRs1 := int64(rs1)

	imm := uint64(inst >> 20)
	signedImm := signExtend(imm, 12)
	uimm := imm & 0x1F

	var result uint64

	switch funct3 {
	case 0x0: // add immediate word
		result = uint64(signExtend(uint64(signedRs1+signedImm), 32))
	case 0x1:
		if funct7 == 0 {
			// shift left logical immediate word
			result = uint64(signExtend(rs1<<uimm, 32))
		} else {
			panic("oops")
		}
	case 0x5:
		if funct7 == 0x00 {
			result = uint64(signExtend((rs1&0xFFFFFFFF)>>uimm, 32))
		} else if funct7 == 0x20 {
			result = uint64(signExtend(uint64(int32(rs1&0xFFFFFFFF)>>uimm), 32))
		} else {
			panic("oops")
		}
	default:
		panic("oops")
	}
	cpu.setRd(inst, result)
}

func (cpu *CpuT) executeBranch(inst uint32) int64 {
	// B format
	// imm7:7 rs2:5 rs1:5 funct3:3 imm:5 op:7
	// The immediate bits are a bit scrambled.

	funct3 := (inst >> 12) & 0x7
	rs1 := cpu.Registers[(inst>>15)&0x1F]
	rs2 := cpu.Registers[(inst>>20)&0x1F]
	signedRs1 := int64(rs1)
	signedRs2 := int64(rs2)

	var branch bool

	switch funct3 {
	case 0x0: // BEQ
		branch = rs1 == rs2
	case 0x1: // BNE
		branch = rs1 != rs2
	case 0x4: // BLT
		branch = signedRs1 < signedRs2
	case 0x5: // BGE
		branch = signedRs1 >= signedRs2
	case 0x6: // BLTU
		branch = rs1 < rs2
	case 0x7: // BGEU
		branch = rs1 >= rs2
	default:
		panic("oops")
	}

	if branch {
		return signExtend(uint64(
			((inst>>7)&0x1E)|
				((inst>>20)&0x7E0)|
				((inst<<4)&0x800)|
				((inst>>19)&0x1000)), 13)
	} else {
		return 4
	}
}

type csrT struct {
	name         string
	number       uint64
	initialValue uint64
	// writable flag?
	// write action?
}

var csrs = map[uint64]*csrT{}
var csrsByName = map[string]*csrT{}

func addCsr(number uint64, name string, initialValue uint64) {
	csr := &csrT{name, number, initialValue}
	csrs[number] = csr
	csrsByName[name] = csr
}

func init() {
	// hardware thread ID.  Must be unique, there must be one zero ID.
	addCsr(0xf14, "mhartid", 0)
	// machine trap handler base address
	addCsr(0x305, "mtvec", 0)
	// resumable NMI status (NonMaskable Interrupt)
	addCsr(0x744, "mnstatus", 0)
	// supervisor address translation and protection
	addCsr(0x180, "satp", 0)
	// physical memory protection address register
	addCsr(0x3b0, "pmpaddr0", 0)
	// physical memory protection configuration
	addCsr(0x3a0, "pmpcfg0", 0)
	// machine interrupt enable register
	addCsr(0x304, "mie", 0)
	// machine exception delegation (a bunch of flags)
	addCsr(0x302, "medeleg", 0)
	// machine interrupt delegation (a bunch of flags)
	addCsr(0x303, "mideleg", 0)
	// supervisor trap vector
	addCsr(0x105, "stvec", 0)
	// machine status
	addCsr(0x300, "mstatus", 0)
	// machine exception program counter
	addCsr(0x341, "mepc", 0)
	// scratch register not interpreted by hardware,
	// typically used for handling traps between modes
	addCsr(0x340, "mscratch", 0)
}

func (cpu *CpuT) executeSystemCall(inst uint32) uint64 {
	// I format
	// imm:12 rs1:5 funct3:3 rd:5 op:7
	imm := uint64(inst >> 20)

	if imm == 0x302 { // mret
		return cpu.csrValues[csrsByName["mepc"]]
	} else if imm != 0x000 {
		panic(fmt.Sprintf("Unknown system call %03X at %08X", imm, cpu.Pc))
	} else if cpu.Registers[arg7] == 93 {
		// Exit2 (terminate with value)
		cpu.Status = cpuExited
		cpu.ExitStatus = uint8(cpu.Registers[arg0])
		return cpu.Pc + 4
	} else if cpu.Registers[arg7] == 1 {
		value := cpu.Registers[arg0]
		cpu.Output = append(cpu.Output, value)
		fmt.Printf("print(%d)\n", value)
		return cpu.Pc + 4
	} else {
		// List of syscalls, including writing integers and such like:
		// https://asm-editor.specy.app/documentation/risc-v/syscall
		// I don't know how official these are or what platforms
		// support them.
		// Linux syscalls are listed here:
		// https://jborza.com/post/2021-05-11-riscv-linux-syscalls/
		// Linux syscalls for lots of architectures:
		// https://gpages.juszkiewicz.com.pl/syscalls-table/syscalls.html
		// riscv64 looks to have the same codes as arm64.
		panic(fmt.Sprintf("Unknown ecall %d", cpu.Registers[arg7]))
	}
}

func (cpu *CpuT) executeCsrReadWrite(inst uint32) {
	// I format
	// imm:12 rs1:5 funct3:3 rd:5 op:7
	funct3 := (inst >> 12) & 0x7
	rs1Field := (inst >> 15) & 0x1F
	rs1 := cpu.Registers[rs1Field]
	imm5 := uint64(rs1Field)
	imm := uint64(inst >> 20)

	var result uint64
	csr := csrs[imm]
	if csr == nil {
		panic(fmt.Sprintf("No CSR for 0x%X (inst %08X)\n", imm, inst))
	}
	value, found := cpu.csrValues[csr]
	if !found {
		value = csr.initialValue
	}
	switch funct3 {
	case 1:
		result = rs1
	case 2:
		result = value | rs1
	case 3:
		result = value & ^rs1
	case 4:
		panic("CSR with func3 == 4")
	case 5:
		result = imm5
	case 6:
		result = value | imm5
	case 7:
		result = value & ^imm5
	}
	cpu.setRd(inst, value)
	cpu.csrValues[csr] = result
}

func (cpu *CpuT) executeLoad(inst uint32) {
	imm := signExtend(uint64(inst>>20), 12)
	address := cpu.Registers[(inst>>15)&0x1F] + uint64(imm)
	var value uint64
	funct3 := (inst >> 12) & 0x7
	switch funct3 {
	case 0x0: // LB
		value = uint64(signExtend(uint64(cpu.readByte(address)), 8))
	case 0x1: // LH
		value = uint64(signExtend(uint64(cpu.readHalfWord(address)), 16))
	case 0x2: // LW
		value = uint64(signExtend(uint64(cpu.readWord(address)), 32))
	case 0x3: // LD
		value = cpu.readDoubleWord(address)
	case 0x4: // LBU
		value = uint64(cpu.readByte(address))
	case 0x5: // LHU
		value = uint64(cpu.readHalfWord(address))
	case 0x6: // LWU
		value = uint64(cpu.readWord(address))
	default:
		panic("oops")
	}
	cpu.setRd(inst, value)
}

func (cpu *CpuT) executeStore(inst uint32) {
	// S format
	// imm:7 rs2:5 rs1:5 funct3:3 imm:5 op:7
	// funct3  0=byte 1=half 2=word
	imm := signExtend(uint64(((inst>>7)&0x1F)|((inst>>25)<<5)), 12)
	address := cpu.Registers[(inst>>15)&0x1F] + uint64(imm)
	value := cpu.Registers[(inst>>20)&0x1F]
	funct3 := (inst >> 12) & 0x7
	switch funct3 {
	case 0x0: // SB
		cpu.writeByte(address, uint8(value))
	case 0x1: // SH
		cpu.writeHalfWord(address, uint16(value))
	case 0x2: // SW
		cpu.writeWord(address, uint32(value))
	case 0x3: // SD
		cpu.writeDoubleWord(address, value)
	default:
		panic("oops")
	}
}

//----------------------------------------------------------------
// Returns a pointer to the bytes at that address.

func (cpu *CpuT) addressLookup(address uint64) []uint8 {
	memory, err := cpu.mustAddressLookup(address)
	if err != nil {
		panic(err)
	}
	return memory
}

func (cpu *CpuT) mustAddressLookup(address uint64) ([]uint8, error) {
	if address < MemoryStart {
		return nil, fmt.Errorf("address before start %X", address)
	}
	address -= MemoryStart
	blockIndex := address >> 16
	if len(cpu.memory) <= int(blockIndex) {
		return nil, fmt.Errorf("block index too large %d", blockIndex)
	}
	block := cpu.memory[blockIndex]
	if block == nil {
		return nil, fmt.Errorf("block index not initialized %d", blockIndex)
	}
	return block[address&0xFFFF:], nil
}

func (cpu *CpuT) CopyToMemory(address uint64, data []uint8) {
	i := 0
	for i < len(data) {
		memory := cpu.addressLookup(address + uint64(i))
		copy(memory, data[i:])
		i += len(memory)
	}
	for i, b := range data {
		if b != cpu.readByte(address+uint64(i)) {
			panic(fmt.Sprintf("failed to read %d at %X + %d\n", b, address, i))
		}
	}
}

func (cpu *CpuT) copyFromMemory(address uint64, data []uint8) {
	i := 0
	for i < len(data) {
		memory := cpu.addressLookup(address + uint64(i))
		copy(data[i:], memory)
		i += len(memory)
	}
}

func (cpu *CpuT) readByte(address uint64) uint8 {
	return cpu.addressLookup(address)[0]
}

func (cpu *CpuT) mustReadByte(address uint64) uint8 {
	memory, err := cpu.mustAddressLookup(address)
	if err != nil {
		return 0
	} else {
		return memory[0]
	}
}

func (cpu *CpuT) readHalfWord(address uint64) uint16 {
	bytes := make([]byte, 2)
	cpu.copyFromMemory(address, bytes)
	return binary.LittleEndian.Uint16(bytes)
}

func (cpu *CpuT) readWord(address uint64) uint32 {
	bytes := make([]byte, 4)
	cpu.copyFromMemory(address, bytes)
	return binary.LittleEndian.Uint32(bytes)
}

func (cpu *CpuT) readDoubleWord(address uint64) uint64 {
	bytes := make([]byte, 8)
	cpu.copyFromMemory(address, bytes)
	return binary.LittleEndian.Uint64(bytes)
}

func (cpu *CpuT) writeByte(address uint64, value uint8) {
	cpu.addressLookup(address)[0] = value
}

func (cpu *CpuT) writeHalfWord(address uint64, value uint16) {
	bytes := make([]byte, 2)
	binary.LittleEndian.PutUint16(bytes, value)
	cpu.CopyToMemory(address, bytes)
}

func (cpu *CpuT) writeWord(address uint64, value uint32) {
	bytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(bytes, value)
	cpu.CopyToMemory(address, bytes)
}

func (cpu *CpuT) writeDoubleWord(address uint64, value uint64) {
	bytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(bytes, value)
	cpu.CopyToMemory(address, bytes)
}

func signExtend(value uint64, width int) int64 {
	value = value & ((1 << width) - 1)
	if value>>(width-1) == 0 {
		return int64(value)
	} else {
		return int64(value) | (int64(-1) << width)
	}
}
