// Copyright 2026 Richard Kelsey. All rights reserved.
// See file LICENSE for notices and license.

// Most of the code, and this comment, are courtesy of Claude.
//
// These tests write an ELF file with WriteElfProgram and read it back
// with the standard library's debug/elf and debug/dwarf -- which, like
// elf2.go's own comment about debug/elf, read DWARF but don't write
// it -- to check that what dwarf2.go writes by hand is what those
// packages expect to see.
package emulator

import (
	"debug/dwarf"
	"debug/elf"
	"path/filepath"
	"testing"
)

// A made-up but valid 4-instruction RV64 program; its contents don't
// matter here, only its length.
var testCode = []byte{
	0x13, 0x05, 0xa0, 0x02, // addi a0,zero,42
	0x93, 0x05, 0x10, 0x00, // addi a1,zero,1
	0x93, 0x08, 0xd0, 0x05, // addi a7,zero,93
	0x73, 0x00, 0x00, 0x00, // ecall
}

// A decoded line table entry, trimmed to what the tests check.
type lineEntryT struct {
	pc     uint64
	file   string
	line   int
	endSeq bool
}

// writeAndOpenDwarf writes testCode at address, with the given debug
// info, to a temporary ELF file, then opens the file's DWARF data and
// finds its one compile unit.  It returns a nil *dwarf.Data, and no
// error, if the file has no DWARF info at all (that's not always a
// test failure: it's the expected result when every record is empty).
func writeAndOpenDwarf(t *testing.T, address uint64, lines []LineRecord) (*dwarf.Data, *dwarf.Entry) {
	t.Helper()
	return writeAndOpenDwarfCode(t, address, testCode, lines)
}

// writeAndOpenDwarfCode is writeAndOpenDwarf with the code to match
// lines against given explicitly, for tests that need a different
// number of instructions than testCode has.
func writeAndOpenDwarfCode(t *testing.T, address uint64, code []byte, lines []LineRecord) (*dwarf.Data, *dwarf.Entry) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.elf")
	WriteElfProgram(code, address, address, lines, path)

	f, err := elf.Open(path)
	if err != nil {
		t.Fatalf("elf.Open: %s", err)
	}
	defer f.Close()

	d, err := f.DWARF()
	if err != nil {
		return nil, nil
	}
	return d, firstCompileUnit(t, d)
}

// writeAndReadLines is writeAndOpenDwarf followed by a sequential walk
// of the whole line number program, across every sequence.
func writeAndReadLines(t *testing.T, address uint64, lines []LineRecord) []lineEntryT {
	t.Helper()
	d, cu := writeAndOpenDwarf(t, address, lines)
	if d == nil {
		return nil
	}
	lr, err := d.LineReader(cu)
	if err != nil {
		t.Fatalf("LineReader: %s", err)
	}
	var result []lineEntryT
	var entry dwarf.LineEntry
	for lr.Next(&entry) == nil {
		result = append(result, lineEntryT{entry.Address, entry.File.Name, entry.Line, entry.EndSequence})
	}
	return result
}

// lineAt looks up pc directly, with a fresh LineReader (one that's
// walked past a sequence can't seek backward into it), rather than by
// walking the whole table as writeAndReadLines does.  ok is false if
// pc isn't covered by any sequence -- the gap behavior this file's
// tests are mainly concerned with.
func lineAt(t *testing.T, d *dwarf.Data, cu *dwarf.Entry, pc uint64) (file string, line int, ok bool) {
	t.Helper()
	lr, err := d.LineReader(cu)
	if err != nil {
		t.Fatalf("LineReader: %s", err)
	}
	var entry dwarf.LineEntry
	if err := lr.SeekPC(pc, &entry); err != nil {
		return "", 0, false
	}
	return entry.File.Name, entry.Line, true
}

func firstCompileUnit(t *testing.T, d *dwarf.Data) *dwarf.Entry {
	t.Helper()
	r := d.Reader()
	entry, err := r.Next()
	if err != nil {
		t.Fatalf("Reader.Next: %s", err)
	}
	if entry == nil || entry.Tag != dwarf.TagCompileUnit {
		t.Fatalf("expected a compile unit DIE, got %v", entry)
	}
	return entry
}

func TestLineInfoRoundTrip(t *testing.T) {
	const base = MemoryStart + 0x1000
	lines := []LineRecord{
		{Filename: "foo.c", Line: 10},
		{}, // no debug info for this instruction
		{Filename: "foo.c", Line: 11},
		{Filename: "bar.c", Line: 3}, // a second file
	}
	got := writeAndReadLines(t, base, lines)
	// The gap at base+4 splits the program into two sequences, each
	// ending with its own end_sequence row: one covering just the
	// first instruction, and one covering the last two.
	want := []lineEntryT{
		{base + 0, "foo.c", 10, false},
		{base + 4, "foo.c", 10, true}, // end_sequence, closing out the first run
		{base + 8, "foo.c", 11, false},
		{base + 12, "bar.c", 3, false},
		{base + 16, "bar.c", 3, true}, // end_sequence, at one past the last instruction
	}
	if !lineEntriesEqual(got, want) {
		t.Errorf("got line table %+v, want %+v", got, want)
	}
}

// The gap itself (base+4, between the two runs above) isn't covered by
// either sequence, so looking it up directly must fail -- that's the
// whole point of splitting into sequences at a gap, rather than just
// leaving an address with no row of its own, which would instead read
// back as whatever the preceding row said.
func TestGapInstructionHasNoLineInfo(t *testing.T) {
	const base = MemoryStart + 0x2000
	lines := []LineRecord{
		{Filename: "foo.c", Line: 5},
		{}, // base+4: the gap
		{Filename: "foo.c", Line: 6},
		{Filename: "foo.c", Line: 6},
	}
	d, cu := writeAndOpenDwarf(t, base, lines)
	if d == nil {
		t.Fatal("got no DWARF info")
	}

	if file, line, ok := lineAt(t, d, cu, base); !ok || file != "foo.c" || line != 5 {
		t.Errorf("pc=base: got (%q, %d, %v), want (foo.c, 5, true)", file, line, ok)
	}
	if _, _, ok := lineAt(t, d, cu, base+4); ok {
		t.Errorf("pc=base+4 (the gap): got a line info entry, want none")
	}
	if file, line, ok := lineAt(t, d, cu, base+8); !ok || file != "foo.c" || line != 6 {
		t.Errorf("pc=base+8: got (%q, %d, %v), want (foo.c, 6, true)", file, line, ok)
	}
	// One past the end of the code is outside every sequence too.
	if _, _, ok := lineAt(t, d, cu, base+16); ok {
		t.Errorf("pc=base+16 (past the end): got a line info entry, want none")
	}
}

// A run of more than one consecutive gap instruction is still just one
// gap -- not, say, one gap per missing instruction -- and the runs on
// either side of it are unaffected.
func TestMultiInstructionGap(t *testing.T) {
	const base = MemoryStart + 0x2000
	lines := []LineRecord{
		{Filename: "foo.c", Line: 1},
		{},
		{},
		{Filename: "foo.c", Line: 2},
	}
	d, cu := writeAndOpenDwarf(t, base, lines)
	if d == nil {
		t.Fatal("got no DWARF info")
	}
	for _, pc := range []uint64{base + 4, base + 8} {
		if _, _, ok := lineAt(t, d, cu, pc); ok {
			t.Errorf("pc=%#x (in the gap): got a line info entry, want none", pc)
		}
	}
	if file, line, ok := lineAt(t, d, cu, base+12); !ok || file != "foo.c" || line != 2 {
		t.Errorf("pc=base+12: got (%q, %d, %v), want (foo.c, 2, true)", file, line, ok)
	}
}

// A leading or trailing gap -- debug info that doesn't start at the
// first instruction, or doesn't reach the last one -- works the same
// way as a gap in the middle.
func TestLeadingAndTrailingGaps(t *testing.T) {
	const base = MemoryStart + 0x2000
	lines := []LineRecord{
		{}, // leading gap
		{Filename: "foo.c", Line: 1},
		{Filename: "foo.c", Line: 2},
		{}, // trailing gap
	}
	d, cu := writeAndOpenDwarf(t, base, lines)
	if d == nil {
		t.Fatal("got no DWARF info")
	}
	if _, _, ok := lineAt(t, d, cu, base); ok {
		t.Errorf("pc=base (leading gap): got a line info entry, want none")
	}
	if file, line, ok := lineAt(t, d, cu, base+4); !ok || file != "foo.c" || line != 1 {
		t.Errorf("pc=base+4: got (%q, %d, %v), want (foo.c, 1, true)", file, line, ok)
	}
	if _, _, ok := lineAt(t, d, cu, base+12); ok {
		t.Errorf("pc=base+12 (trailing gap): got a line info entry, want none")
	}
}

func TestNoDebugInfoAtAll(t *testing.T) {
	const base = MemoryStart + 0x3000
	for _, lines := range [][]LineRecord{
		nil,
		make([]LineRecord, len(testCode)/4), // all zero-value, i.e. all Filename==""
	} {
		got := writeAndReadLines(t, base, lines)
		if got != nil {
			t.Errorf("lines=%v: got a line table %+v, want none", lines, got)
		}
	}
}

func TestMismatchedLineCountPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("expected a panic for a lines slice of the wrong length")
		}
	}()
	path := filepath.Join(t.TempDir(), "test.elf")
	WriteElfProgram(testCode, MemoryStart, MemoryStart, []LineRecord{{Filename: "foo.c", Line: 1}}, path)
}

// A run of instructions that all say the same file and line should
// collapse to a single row, rather than one row per instruction: the
// whole point of the run-length compression in appendLineSequence.
func TestRepeatedLineIsCompressed(t *testing.T) {
	const base = MemoryStart + 0x4000
	code := make([]byte, 5*4)
	lines := []LineRecord{
		{Filename: "foo.c", Line: 1},
		{Filename: "foo.c", Line: 1}, // repeats the row before it
		{Filename: "foo.c", Line: 1}, // and again
		{Filename: "foo.c", Line: 2}, // a real change
		{Filename: "foo.c", Line: 2}, // repeats that one
	}
	d, cu := writeAndOpenDwarfCode(t, base, code, lines)
	if d == nil {
		t.Fatal("got no DWARF info")
	}

	// Every address reads back correctly, repeated or not.
	want := map[uint64]int{base: 1, base + 4: 1, base + 8: 1, base + 12: 2, base + 16: 2}
	for pc, wantLine := range want {
		if file, line, ok := lineAt(t, d, cu, pc); !ok || file != "foo.c" || line != wantLine {
			t.Errorf("pc=%#x: got (%q, %d, %v), want (foo.c, %d, true)", pc, file, line, ok, wantLine)
		}
	}

	// But the table itself has only two rows (one for line 1, one for
	// line 2) plus the end_sequence, not one per instruction.
	lr, err := d.LineReader(cu)
	if err != nil {
		t.Fatalf("LineReader: %s", err)
	}
	var got []lineEntryT
	var entry dwarf.LineEntry
	for lr.Next(&entry) == nil {
		got = append(got, lineEntryT{entry.Address, entry.File.Name, entry.Line, entry.EndSequence})
	}
	wantRows := []lineEntryT{
		{base, "foo.c", 1, false},
		{base + 12, "foo.c", 2, false},
		{base + 20, "foo.c", 2, true},
	}
	if !lineEntriesEqual(got, wantRows) {
		t.Errorf("got line table %+v, want %+v", got, wantRows)
	}
}

func lineEntriesEqual(got, want []lineEntryT) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
