// Copyright 2026 Richard Kelsey. All rights reserved.
// See file LICENSE for notices and license.

// Most of the code, and the comment below, are courtesy of Claude.
//
// Minimal DWARF (version 4) line-number debug info for the code
// WriteElfProgram writes out, so a debugger or addr2line can map an
// instruction's address back to a source file and line.
//
// The Go standard library's debug/dwarf package reads DWARF but, like
// debug/elf, doesn't write it; that package is used to check this
// code's output in dwarf2_test.go.
//
// What's written is deliberately small: one compile unit, one
// .debug_line sequence, and no .debug_str, .debug_aranges or
// .debug_frame section -- all the strings (file names, the compile
// unit's name, and the producer string) are written inline with
// DW_FORM_string instead.  There's no attempt to use DWARF's special
// opcodes, which pack a line and address advance into a single byte;
// every row is instead the straightforward (and much more verbose)
// set-file / advance-line / advance-pc / copy sequence, since this
// isn't a compiler trying to keep its output small.  A row is skipped
// when it would say the same file and line as the row before it --
// since a row's file and line apply to every address up to the next
// row (or the end of the sequence), repeating one changes nothing but
// the table's size, and source code commonly has several consecutive
// instructions on one line.  A LineRecord with an empty Filename means
// that instruction has no debug info; a maximal run of consecutive
// instructions that do have it becomes its own DWARF sequence (its own
// DW_LNE_set_address / DW_LNE_end_sequence pair), so an address with no
// debug info is a real gap, covered by no sequence at all, rather than
// silently inheriting whatever the preceding row said.

package emulator

import "slices"

// LineRecord associates one instruction with the source file and line
// it came from.  An empty Filename means there's no debug information
// for that instruction.
type LineRecord struct {
	Filename string
	Line     int
}

// DWARF tags, attributes and forms.  See the DWARF Debugging
// Information Format Version 4 standard for the full set; this is
// only the handful used below.
const (
	dwTagCompileUnit = 0x11

	dwAtName     = 0x03
	dwAtStmtList = 0x10
	dwAtLowpc    = 0x11
	dwAtHighpc   = 0x12
	dwAtCompDir  = 0x1b
	dwAtProducer = 0x25

	dwFormAddr      = 0x01
	dwFormString    = 0x08
	dwFormSecOffset = 0x17

	dwChildrenNo = 0x00

	// Line number program standard opcodes.
	dwLnsCopy        = 1
	dwLnsAdvancePc   = 2
	dwLnsAdvanceLine = 3
	dwLnsSetFile     = 4

	// Line number program extended opcodes, each preceded by a 0x00
	// byte and a ULEB128 length, unlike the standard opcodes above.
	dwLneEndSequence = 1
	dwLneSetAddress  = 2
)

// addDwarfLineInfo adds DWARF debug info to file mapping the code
// loaded at address, instruction by instruction, to the source files
// and lines named in lines.  lines may be nil, or all-empty, meaning
// there's no debug info to add, in which case file is left unchanged.
func addDwarfLineInfo(file *File, address uint64, lines []LineRecord) {
	firstFile := ""
	for _, line := range lines {
		if line.Filename != "" {
			firstFile = line.Filename
			break
		}
	}
	if firstFile == "" {
		return
	}

	highpc := address + uint64(len(lines))*4
	file.Sections = append(file.Sections,
		&Section{Name: ".debug_abbrev", Data: buildDebugAbbrev()},
		&Section{Name: ".debug_info", Data: buildDebugInfo(address, highpc, firstFile)},
		&Section{Name: ".debug_line", Data: buildDebugLine(address, lines)})
}

// buildDebugAbbrev returns the .debug_abbrev section data: the single
// abbreviation declaration buildDebugInfo's compile unit DIE uses.
func buildDebugAbbrev() []byte {
	var buf []byte
	buf = appendUleb128(buf, 1) // abbreviation code, referenced from buildDebugInfo
	buf = appendUleb128(buf, dwTagCompileUnit)
	buf = append(buf, dwChildrenNo)
	attr := func(at, form uint64) {
		buf = appendUleb128(buf, at)
		buf = appendUleb128(buf, form)
	}
	attr(dwAtName, dwFormString)
	attr(dwAtCompDir, dwFormString)
	attr(dwAtLowpc, dwFormAddr)
	attr(dwAtHighpc, dwFormAddr)
	attr(dwAtStmtList, dwFormSecOffset)
	attr(dwAtProducer, dwFormString)
	buf = appendUleb128(buf, 0) // end of this abbreviation's attributes
	buf = appendUleb128(buf, 0)
	buf = appendUleb128(buf, 0) // end of the abbreviation table
	return buf
}

// buildDebugInfo returns the .debug_info section data: a header
// followed by a single compile-unit DIE, using abbreviation code 1
// from buildDebugAbbrev, with no children.  lowpc and highpc are the
// address range the code covers; highpc is one past the last byte,
// following the DW_FORM_addr convention used here (as opposed to the
// DW_FORM_data* convention, where it would be a length).  name is
// used as both the compile unit's name and (since nothing better is
// available) its producer string; the compilation directory is left
// empty.
func buildDebugInfo(lowpc, highpc uint64, name string) []byte {
	var die []byte
	die = appendUleb128(die, 1) // abbreviation code
	die = appendCString(die, name)
	die = appendCString(die, "") // DW_AT_comp_dir
	die = append(die, uint64Bytes(lowpc)...)
	die = append(die, uint64Bytes(highpc)...)
	die = append(die, uint32Bytes(0)...) // DW_AT_stmt_list: the only
	// line number program, at the start of .debug_line
	die = appendCString(die, "riscv-emulator") // DW_AT_producer

	body := slices.Concat(
		uint16Bytes(4), // version
		uint32Bytes(0), // debug_abbrev_offset: the only abbrev table
		[]byte{8},      // address_size
		die)
	return slices.Concat(uint32Bytes(uint32(len(body))), body)
}

// buildDebugLine returns the .debug_line section data: a header
// followed by a line number program that maps the code loaded at
// address, instruction by instruction, to the source files and lines
// named in lines (see LineRecord).  The program has one DWARF
// sequence per maximal run of consecutive instructions that have
// debug info; an instruction with none falls in the gap between two
// sequences (or before the first or after the last), which is how
// DWARF says "no information here" as opposed to "same as before".
//
// Every RV64 instruction the emulator runs is 4 bytes, so consecutive
// LineRecords are always 4 bytes apart; that's hardcoded in both the
// line_range calculation below and the address advances in the
// program itself.
func buildDebugLine(address uint64, lines []LineRecord) []byte {
	fileIndex := map[string]uint64{}
	var fileNames []string
	for _, line := range lines {
		if line.Filename == "" {
			continue
		}
		if _, ok := fileIndex[line.Filename]; !ok {
			fileIndex[line.Filename] = uint64(len(fileNames) + 1) // DWARF <= 4 numbers files from 1
			fileNames = append(fileNames, line.Filename)
		}
	}

	// The line number program header, DWARF4 style: everything up to
	// (and not including) the program itself.  header_length below is
	// the length of this, from directly after the header_length field
	// to the start of the program.
	var header []byte
	header = append(header,
		1,    // minimum_instruction_length: no scaling of address advances
		1,    // maximum_operations_per_instruction
		1,    // default_is_stmt
		0xfb, // line_base: -5, as an unsigned byte
		14,   // line_range
		13)   // opcode_base: standard opcodes are 1 through 12
	header = append(header, 0, 1, 1, 1, 1, 0, 0, 0, 1, 0, 0, 1) // standard_opcode_lengths, for opcodes 1-12
	header = append(header, 0)                                  // include_directories: none, just the terminator
	for _, name := range fileNames {
		header = appendCString(header, name)
		header = appendUleb128(header, 0) // directory index: the compilation directory
		header = appendUleb128(header, 0) // mtime: unknown
		header = appendUleb128(header, 0) // length: unknown
	}
	header = append(header, 0) // file_names terminator

	// The line number program: one sequence per maximal run of
	// consecutive instructions that have debug info.  Indices with no
	// debug info are simply skipped over between runs, leaving a true
	// gap -- no sequence covers them.
	var program []byte
	i := 0
	for i < len(lines) {
		if lines[i].Filename == "" {
			i++
			continue
		}
		start := i
		for i < len(lines) && lines[i].Filename != "" {
			i++
		}
		program = appendLineSequence(program, address, lines, fileIndex, start, i)
	}

	body := slices.Concat(
		uint16Bytes(4), // version
		uint32Bytes(uint32(len(header))),
		header,
		program)
	return slices.Concat(uint32Bytes(uint32(len(body))), body)
}

// appendLineSequence appends one DWARF line number program sequence to
// buf, covering lines[start:end] (a maximal run of instructions that
// all have debug info -- the caller finds the run; this just encodes
// it).  Each sequence's registers start fresh, per the DWARF spec: the
// address register at the run's first instruction, file at 1, line at
// 1.  file starts at 0 here instead of the official 1, so that the
// first record in the run (which is usually, but not necessarily, file
// 1) still causes an explicit DW_LNS_set_file; it's always safe to set
// a register to its current value, just slightly wasteful.
func appendLineSequence(buf []byte, address uint64, lines []LineRecord, fileIndex map[string]uint64, start, end int) []byte {
	runAddr := address + uint64(start)*4
	buf = appendSetAddress(buf, runAddr)
	file := uint64(0)
	line := 1
	addr := runAddr
	for i := start; i < end; i++ {
		rec := lines[i]
		idx := fileIndex[rec.Filename]
		if idx == file && rec.Line == line {
			// Same file and line as the row just emitted: the
			// address range up to here is already covered by that
			// row, so there's nothing new to say.  This is the run-
			// length compression described at the top of the file.
			continue
		}
		want := address + uint64(i)*4
		if want != addr {
			buf = append(buf, dwLnsAdvancePc)
			buf = appendUleb128(buf, want-addr)
			addr = want
		}
		if idx != file {
			buf = append(buf, dwLnsSetFile)
			buf = appendUleb128(buf, idx)
			file = idx
		}
		if rec.Line != line {
			buf = append(buf, dwLnsAdvanceLine)
			buf = appendSleb128(buf, int64(rec.Line-line))
			line = rec.Line
		}
		buf = append(buf, dwLnsCopy)
	}
	// End the sequence one instruction past the end of the run.
	runEnd := address + uint64(end)*4
	buf = append(buf, dwLnsAdvancePc)
	buf = appendUleb128(buf, runEnd-addr)
	return appendEndSequence(buf)
}

// appendSetAddress appends a DW_LNE_set_address extended opcode,
// setting the line number program's address register.
func appendSetAddress(buf []byte, address uint64) []byte {
	buf = append(buf, 0)
	buf = appendUleb128(buf, 9) // the sub-opcode byte plus an 8-byte address
	buf = append(buf, dwLneSetAddress)
	return append(buf, uint64Bytes(address)...)
}

// appendEndSequence appends a DW_LNE_end_sequence extended opcode,
// which closes out the current sequence at the address register's
// current value.
func appendEndSequence(buf []byte) []byte {
	buf = append(buf, 0)
	buf = appendUleb128(buf, 1) // just the sub-opcode byte
	return append(buf, dwLneEndSequence)
}

// appendCString appends s to buf as a NUL-terminated string, DWARF's
// DW_FORM_string.
func appendCString(buf []byte, s string) []byte {
	buf = append(buf, s...)
	return append(buf, 0)
}

// appendUleb128 appends value to buf as unsigned little-endian
// base-128 (DWARF's DW_FORM_udata and friends): seven bits per byte,
// least significant group first, with the top bit of each byte set
// except on the last one.
func appendUleb128(buf []byte, value uint64) []byte {
	for {
		b := byte(value & 0x7f)
		value >>= 7
		if value != 0 {
			buf = append(buf, b|0x80)
		} else {
			return append(buf, b)
		}
	}
}

// appendSleb128 appends value to buf as signed little-endian base-128
// (DWARF's DW_FORM_sdata): like appendUleb128, but sign-extended, and
// stopping once the remaining sign-extended bits are redundant with
// the last byte's sign bit.
func appendSleb128(buf []byte, value int64) []byte {
	for {
		b := byte(value & 0x7f)
		value >>= 7 // arithmetic shift: sign-extending
		signBit := b&0x40 != 0
		if (value == 0 && !signBit) || (value == -1 && signBit) {
			return append(buf, b)
		}
		buf = append(buf, b|0x80)
	}
}
