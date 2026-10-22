// Copyright 2026 Richard Kelsey. All rights reserved.
// See file LICENSE for notices and license.

package emulator

import (
	"debug/elf"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"slices"
)

// The debug/elf package only reads ELF files and does not write them.

type File struct {
	EntryPoint uint64
	Progs      []*Prog
	// Named sections with no program header of their own, i.e. not
	// loaded into memory when the program runs.  dwarf2.go uses these
	// to attach debug info; there's nothing else here that writes one.
	Sections []*Section
}

// A named chunk of data written as an ELF section but, having no
// corresponding Prog, not loaded into memory -- for data meant for a
// tool working with the file, such as a debugger, rather than for the
// program itself.
type Section struct {
	Name string
	Data []uint8
}

// progs:
// This first one is PT_RISCV_ATTRIBUTES, which may be deprecated.
//   type PT_LOPROC+3 flags PF_R size 99 address 0
//   type PT_LOAD flags PF_X+PF_R size 1724 address 80000000
//   type PT_LOAD flags PF_W+PF_R size 72 address 80001000

/*
  Elf64_Word	p_type;			// Segment type
  Elf64_Word	p_flags;		// Segment flags
  Elf64_Off	p_offset;		// Segment file offset
  Elf64_Addr	p_vaddr;		// Segment virtual address
  Elf64_Addr	p_paddr;		// Segment physical address
  Elf64_Xword	p_filesz;		// Segment size in file
  Elf64_Xword	p_memsz;		// Segment size in memory
  Elf64_Xword	p_align;		// Segment alignment
*/

// 01 00 00 00   type
// 05 00 00 00   flags = PF_X PF_R
// 00 10 00 00 00 00 00 00 file offset
// 00 00 00 80 00 00 00 00 virtual address offset
// 00 00 00 80 00 00 00 00 physical address offset
// bc 06 00 00 00 00 00 00 size in file
// bc 06 00 00 00 00 00 00 size in memory
// 00 10 00 00 00 00 00 00 alignment = 0x1000 page alignment

type Prog struct {
	Type  elf.ProgType
	Flags elf.ProgFlag
	// Off    uint64  // offset in elf file
	Vaddr uint64 // virtual memory address
	// Paddr  uint64  // physical memory address - same as virtual
	Filesz uint64 // size in file
	Memsz  uint64 // size in memory
	//Align  uint64  // alignment in bits - always 64
	Data []uint8
}

// 00 00 00 80 00 00 00 00 entry point
// 40 00 00 00 00 00 00 00 program header file offset
// 50 27 00 00 00 00 00 00 section header file offset
// 00 00 00 00 architecture-specifc flags
// 40 00 elf header size
// 38 00 size of program header
// 03 00 number of program headers
// 40 00 size of section header
// 07 00 number of section headers
// 06 00 section names string section

var fileHeader = []uint8{
	0x7f, 0x45, 0x4c, 0x46, // DEL E L F
	0x02,                                     // class = 64 bit
	0x01,                                     // data format = 2's complement, little endian
	0x01,                                     // ELF format version = current
	0x00,                                     // OSABI (operating system) = None/UNIX System V AB
	0x00,                                     // ABI version
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // padding
	0x02, 0x00, // type = executable
	0xf3, 0x00, // machine = RISCV
	0x01, 0x00, 0x00, 0x00} // object file version

const progAlign = 0x1000 // page alignment for prog data

func alignProg(offset int) int {
	delta := offset % progAlign
	if delta == 0 {
		return 0
	}
	return progAlign - delta
}

func WriteElf(file *File, filename string) {
	const ehdrSize = 64
	const phdrSize = 56
	const shdrSize = 64

	offset := ehdrSize + phdrSize*len(file.Progs)
	// program headers
	programHeaderBlocks := [][]byte{}
	programDataBlocks := [][]byte{}
	progDataOffsets := make([]int, len(file.Progs))
	for i, prog := range file.Progs {
		delta := alignProg(offset)
		offset += delta
		programDataBlocks = append(programDataBlocks, make([]byte, delta))
		header := slices.Concat(
			uint32Bytes(uint32(prog.Type)),
			uint32Bytes(uint32(prog.Flags)),
			uint64Bytes(uint64(offset)),
			uint64Bytes(prog.Vaddr),
			uint64Bytes(prog.Vaddr),
			uint64Bytes(uint64(len(prog.Data))),
			uint64Bytes(uint64(len(prog.Data))), // + prog.NumExtraBytes),
			uint64Bytes(progAlign))
		programHeaderBlocks = append(programHeaderBlocks, header)
		programDataBlocks = append(programDataBlocks, prog.Data)
		progDataOffsets[i] = offset
		offset += len(prog.Data)
	}

	// Sections.  If file.Sections is empty, the file has no section
	// headers at all, matching how this function has always written
	// files with just program headers.  Otherwise, alongside
	// file.Sections (debug info and the like, not loaded into memory,
	// so with no program header), we also add one loaded section per
	// Prog, describing data already written above as program data,
	// not any new bytes.  Those aren't needed to run the program --
	// the program headers alone do that -- but a tool that locates
	// debug info by first finding which section covers an address, as
	// addr2line and gdb do, can't find ours without them.
	sectionDataBlocks := [][]byte{}
	sectionHeaderBlocks := [][]byte{}
	shoff := 0
	shnum := 0
	shstrndx := 0
	if len(file.Sections) > 0 {
		type loadedSection struct {
			name       string
			flags      elf.SectionFlag
			addr       uint64
			dataOffset int
			size       int
		}
		var progSections []loadedSection
		usedNames := map[string]bool{}
		for i, prog := range file.Progs {
			name := ".rodata"
			flags := elf.SHF_ALLOC
			if prog.Flags&elf.PF_X != 0 {
				name = ".text"
				flags |= elf.SHF_EXECINSTR
			} else if prog.Flags&elf.PF_W != 0 {
				name = ".data"
				flags |= elf.SHF_WRITE
			}
			if usedNames[name] {
				name = fmt.Sprintf("%s.%d", name, i)
			}
			usedNames[name] = true
			progSections = append(progSections,
				loadedSection{name, flags, prog.Vaddr, progDataOffsets[i], len(prog.Data)})
		}

		// The section header string table: a leading NUL (for the
		// empty name of the NULL section header), then each
		// section's name, then this table's own name.
		shstrtab := []byte{0}
		progNameOffsets := make([]int, len(progSections))
		for i, section := range progSections {
			progNameOffsets[i] = len(shstrtab)
			shstrtab = append(shstrtab, section.name...)
			shstrtab = append(shstrtab, 0)
		}
		debugNameOffsets := make([]int, len(file.Sections))
		for i, section := range file.Sections {
			debugNameOffsets[i] = len(shstrtab)
			shstrtab = append(shstrtab, section.Name...)
			shstrtab = append(shstrtab, 0)
		}
		shstrtabNameOffset := len(shstrtab)
		shstrtab = append(shstrtab, ".shstrtab"...)
		shstrtab = append(shstrtab, 0)

		pad := func() {
			delta := alignProg8(offset)
			offset += delta
			sectionDataBlocks = append(sectionDataBlocks, make([]byte, delta))
		}

		sectionHeader := func(nameOffset int, shType elf.SectionType, flags elf.SectionFlag, addr uint64, dataOffset, size int) []byte {
			return slices.Concat(
				uint32Bytes(uint32(nameOffset)),
				uint32Bytes(uint32(shType)),
				uint64Bytes(uint64(flags)),
				uint64Bytes(addr),
				uint64Bytes(uint64(dataOffset)),
				uint64Bytes(uint64(size)),
				uint32Bytes(0), // link
				uint32Bytes(0), // info
				uint64Bytes(1), // addralign
				uint64Bytes(0)) // entsize
		}

		// Section header 0 is reserved, and must be all zero.
		sectionHeaderBlocks = append(sectionHeaderBlocks, make([]byte, shdrSize))
		for i, section := range progSections {
			sectionHeaderBlocks = append(sectionHeaderBlocks,
				sectionHeader(progNameOffsets[i], elf.SHT_PROGBITS, section.flags, section.addr,
					section.dataOffset, section.size))
		}
		for i, section := range file.Sections {
			pad()
			sectionDataBlocks = append(sectionDataBlocks, section.Data)
			sectionHeaderBlocks = append(sectionHeaderBlocks,
				sectionHeader(debugNameOffsets[i], elf.SHT_PROGBITS, 0, 0, offset, len(section.Data)))
			offset += len(section.Data)
		}
		pad()
		shstrtabOffset := offset
		sectionDataBlocks = append(sectionDataBlocks, shstrtab)
		offset += len(shstrtab)
		sectionHeaderBlocks = append(sectionHeaderBlocks,
			sectionHeader(shstrtabNameOffset, elf.SHT_STRTAB, 0, 0, shstrtabOffset, len(shstrtab)))

		pad()
		shoff = offset
		shnum = len(progSections) + len(file.Sections) + 2 // the reserved entry and .shstrtab
		shstrndx = shnum - 1                               // .shstrtab is always last
	}

	execHeader := slices.Concat(
		uint64Bytes(file.EntryPoint),
		uint64Bytes(uint64(ehdrSize)), // program header offset
		uint64Bytes(uint64(shoff)),
		uint32Bytes(0),                       // architecture-specifc flags
		uint16Bytes(ehdrSize),                // elf header size
		uint16Bytes(phdrSize),                // size of program header
		uint16Bytes(uint16(len(file.Progs))), // number of program headers
		uint16Bytes(shdrSize),                // size of section header
		uint16Bytes(uint16(shnum)),           // number of section headers
		uint16Bytes(uint16(shstrndx)))        // section names string section
	blocks := [][]byte{fileHeader, execHeader}
	blocks = append(blocks, programHeaderBlocks...)
	blocks = append(blocks, programDataBlocks...)
	blocks = append(blocks, sectionDataBlocks...)
	blocks = append(blocks, sectionHeaderBlocks...)

	bytes := slices.Concat(blocks...)
	err := os.WriteFile(filename, bytes, 0644)
	if err != nil {
		fmt.Printf("failed to write %s: %s\n", filename, err)
		return
	}
	fmt.Printf("Wrote '%s'.\n", filename)
}

// alignProg8 is alignProg with an 8-byte alignment, for the data that
// doesn't need page alignment but reads more cleanly aligned anyway.
func alignProg8(offset int) int {
	delta := offset % 8
	if delta == 0 {
		return 0
	}
	return 8 - delta
}

// WriteElfProgram writes an executable ELF file with a single PT_LOAD
// segment holding code, loaded at address, and with the entry point
// set to startAddress.
//
// lines, if not nil, must have one entry per 4-byte instruction in
// code, giving the source file and line it came from; see LineRecord
// in dwarf2.go.  An entry with an empty Filename means there's no
// debug information for that instruction.  DWARF debug info built
// from lines is added to the file, unless every entry is empty, in
// which case there's nothing to add and the file comes out exactly
// as WriteElfProgram always wrote it.
func WriteElfProgram(
	code []uint8,
	address uint64,
	startAddress uint64,
	lines []LineRecord,
	filename string,
) {
	if lines != nil && len(lines) != len(code)/4 {
		panic(fmt.Sprintf("WriteElfProgram: %d lines for %d bytes of code"+
			" (want %d, one per instruction)", len(lines), len(code), len(code)/4))
	}
	prog := &Prog{
		Type:   elf.PT_LOAD,
		Flags:  elf.PF_X | elf.PF_R,
		Vaddr:  address,
		Filesz: uint64(len(code)),
		Memsz:  uint64(len(code)),
		Data:   code,
	}
	file := &File{EntryPoint: startAddress, Progs: []*Prog{prog}}
	addDwarfLineInfo(file, address, lines)
	WriteElf(file, filename)
}

func uint64Bytes(n uint64) []uint8 {
	bytes := make([]uint8, 8)
	binary.LittleEndian.PutUint64(bytes, n)
	return bytes
}

func uint16Bytes(n uint16) []uint8 {
	bytes := make([]uint8, 2)
	binary.LittleEndian.PutUint16(bytes, n)
	return bytes
}

func uint32Bytes(n uint32) []uint8 {
	bytes := make([]uint8, 4)
	binary.LittleEndian.PutUint32(bytes, n)
	return bytes
}

// ConvertElf convert a debug/elf.File to a File, keeping only the
// LOAD progs.
func ConvertElf(file *elf.File) *File {
	progs := []*Prog{}
	for _, prog := range file.Progs {
		if prog.Type == elf.PT_LOAD {
			data, err := io.ReadAll(prog.Open())
			if err != nil {
				panic(fmt.Sprintf("Failed to get prog data: %s", err))
			}
			progs = append(progs, &Prog{
				prog.Type,
				prog.Flags,
				prog.Vaddr,
				prog.Filesz,
				prog.Memsz,
				data})
		}
	}
	return &File{EntryPoint: file.Entry, Progs: progs}
}
