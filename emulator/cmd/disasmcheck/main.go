// disasmcheck checks disassemble.go against the real RISC-V toolchain.
//
// It generates a large synthetic sweep of 32-bit instruction words --
// systematic funct3/funct7 combinations for every opcode disassemble.go
// knows about, field edge cases (all-zero and all-ones registers and
// immediates), and a handful of specific pseudo-instruction encodings --
// assembles them with the real assembler, disassembles the result with
// the real objdump, and compares that against emulator.Disassemble.
//
// Every difference found is required to fall into one of the three
// categories documented at the top of disassemble.go (the floating
// point CSR aliases, unnamed CSRs, and unknown fence flag sets); any
// other difference is reported as a failure.
//
// This needs a RISC-V binutils install (as, objcopy, objdump) on the
// machine; without one, nothing has verified the disassembler, so
// that's a failure too, not a skip.
//
//	go run ./cmd/disasmcheck
package main

import (
	"bufio"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/s48/riscv/emulator"
)

// The opcodes disassemble.go knows how to decode.  This mirrors the
// switch in cpu.go's execute and Disassemble.
var opcodes = []uint32{0x03, 0x0F, 0x13, 0x17, 0x1B, 0x23, 0x33, 0x37, 0x3B, 0x63, 0x67, 0x6F, 0x73}

// A handful of specific encodings, mostly to exercise pseudo-instructions
// that only show up for one particular combination of fields.
var specialCases = []uint32{
	0x00000013, // nop
	0x00050513, // mv a0,a0
	0xfff00513, // li a0,-1
	0x00008067, // ret
	0x000080e7, // jalr ra
	0x00030067, // jr t1
	0x40b00533, // neg a0,a1
	0x40b0053b, // negw a0,a1
	0x00b03533, // snez a0,a1
	0x0005879b, // sext.w a5,a1
	0xfff5c513, // not a0,a1
	0x0015b513, // seqz a0,a1
	0x0000100f, // fence.i
	0x0ff0000f, // fence iorw,iorw
	0x8330000f, // fence.tso
	0x0100000f, // pause
	0x00000073, // ecall
	0x00100073, // ebreak
	0x30200073, // mret
	0xf14022f3, // csrr t0,mhartid
	0x30529073, // csrw mtvec,t0
	0x3002a073, // csrs mstatus,t0
	0x1800b073, // csrc satp,ra
	0x30046073, // csrsi mstatus,8
	0xffe00a63, // beqz zero,-12
	0x7ffff06f, // jal with max positive offset
	0x800000ef, // jal with max negative offset
	0xc0001073, // unimp
	0x34202f73, // csrr t5,mcause
}

// generateInstructions builds the synthetic sweep, deterministically
// (the random source is always seeded the same way) so a failure is
// reproducible.
func generateInstructions() []uint32 {
	var insts []uint32
	add := func(i uint32) { insts = append(insts, i) }

	r := rand.New(rand.NewSource(42))
	reg := func() uint32 { return uint32(r.Intn(32)) }

	// Systematic: every funct3/funct7 combination for each opcode,
	// with random registers and immediates.
	for _, op := range opcodes {
		for funct3 := uint32(0); funct3 < 8; funct3++ {
			for _, funct7 := range []uint32{0x00, 0x01, 0x10, 0x20} {
				for n := 0; n < 12; n++ {
					imm := uint32(r.Intn(1 << 12))
					inst := op | reg()<<7 | funct3<<12 | reg()<<15 | reg()<<20 | funct7<<25
					switch op {
					case 0x17, 0x37, 0x6F: // U and J format: lui, auipc, jal
						inst = op | reg()<<7 | uint32(r.Intn(1<<20))<<12
					case 0x13, 0x1B, 0x03, 0x67: // I format
						inst = op | reg()<<7 | funct3<<12 | reg()<<15 | imm<<20
						if n%3 == 0 {
							// Exercise the shift instructions' split
							// immediate (funct7 high bits, shift count low).
							inst = op | reg()<<7 | funct3<<12 | reg()<<15 | (funct7<<5|uint32(r.Intn(32)))<<20
						}
					case 0x73: // system / CSR
						inst = op | reg()<<7 | funct3<<12 | reg()<<15 | imm<<20
						if funct3 == 0 && n%2 == 0 {
							sys := []uint32{0x000, 0x001, 0x002, 0x102, 0x302, 0x105}
							inst = op | funct3<<12 | sys[n/2%len(sys)]<<20
						}
					}
					add(inst)
				}
			}
		}
	}

	// Field edge cases: all-zero and all-ones register and immediate
	// fields, with every funct3/funct7 combination.  These catch
	// off-by-one errors in field extraction that random values tend
	// to miss.
	for _, op := range opcodes {
		for funct3 := uint32(0); funct3 < 8; funct3++ {
			for _, funct7 := range []uint32{0x00, 0x01, 0x10, 0x20} {
				for _, rd := range []uint32{0, 1, 31} {
					for _, rs := range []uint32{0, 1, 31} {
						for _, imm := range []uint32{0, 1, 0xFFF, 0x800} {
							add(op | rd<<7 | funct3<<12 | rs<<15 | rs<<20 | funct7<<25)
							add(op | rd<<7 | funct3<<12 | rs<<15 | imm<<20)
						}
					}
				}
			}
		}
	}

	insts = append(insts, specialCases...)
	return insts
}

// A disassembled instruction, split into its mnemonic and the raw text
// of its operands (still comma-separated), for comparing against the
// other tool's output.
type decodedT struct {
	word     uint32
	mnemonic string
	operands string
}

func decode(word uint32, text string) decodedT {
	text = strings.TrimSpace(text)
	mnemonic, operands, _ := strings.Cut(text, " ")
	return decodedT{word, mnemonic, strings.TrimSpace(operands)}
}

func (d decodedT) operandList() []string {
	if d.operands == "" {
		return nil
	}
	parts := strings.Split(d.operands, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

func main() {
	toolchain, err := findToolchain()
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %s\n", err)
		os.Exit(1)
	}

	insts := generateInstructions()
	fmt.Printf("generated %d instructions\n", len(insts))

	ours := make([]decodedT, len(insts))
	for i, word := range insts {
		ours[i] = decode(word, emulator.Disassemble(uint64(i)*4, word, nil))
	}

	theirs, err := objdumpDisassemble(toolchain, insts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to run objdump: %s\n", err)
		os.Exit(1)
	}
	if len(theirs) != len(insts) {
		fmt.Fprintf(os.Stderr,
			"objdump produced %d lines for %d instructions; a word must have"+
				" been split or merged\n", len(theirs), len(insts))
		os.Exit(1)
	}

	exceptions := map[string]int{}
	var failures []string
	for i, inst := range insts {
		mine, real := ours[i], theirs[i]
		if category, ok := classify(mine, real); ok {
			if category != "" {
				exceptions[category]++
			}
			continue
		}
		if len(failures) < 40 {
			failures = append(failures, fmt.Sprintf(
				"  %08x: got %q, objdump says %q", inst, mine.mnemonic+" "+mine.operands, real.mnemonic+" "+real.operands))
		} else if len(failures) == 40 {
			failures = append(failures, "  ...")
		}
	}

	for _, category := range []string{"fp CSR alias", "unnamed CSR", "unknown fence flags"} {
		if n := exceptions[category]; n > 0 {
			fmt.Printf("%d instructions differed only by the documented %s exception\n", n, category)
		}
	}

	if len(failures) == 0 {
		fmt.Printf("PASS: all %d instructions matched objdump\n", len(insts))
		return
	}
	fmt.Printf("FAIL: %d instructions disagreed with objdump:\n", len(failures))
	for _, line := range failures {
		fmt.Println(line)
	}
	os.Exit(1)
}

// classify decides whether a difference between our disassembly and
// objdump's is a real bug, or one of the three documented, accepted
// exceptions (see the comment at the top of disassemble.go).  It
// returns ok==true, with the exception's name (or "" if the two
// outputs actually agree), when the difference is accepted.
func classify(mine, real decodedT) (category string, ok bool) {
	if mine.mnemonic == real.mnemonic && mine.operands == real.operands {
		return "", true
	}

	// Both sides failed to decode it.
	if strings.HasPrefix(mine.mnemonic, ".word") && strings.HasPrefix(real.mnemonic, ".insn") {
		return "", true
	}

	// Exception 1: the floating point CSR aliases.  Our mnemonic is
	// csr[rwsc]*[i]? with "fflags", "frm" or "fcsr" as the CSR name;
	// objdump drops the CSR name and uses one of its own mnemonics.
	// "csrr..." names rd explicitly, at operands[0], with the CSR name
	// at operands[1]; the plain "csr..." forms (used when rd is
	// discarded) have just the CSR name, at operands[0], and no rd.
	if strings.HasPrefix(mine.mnemonic, "csr") {
		operands := mine.operandList()
		nameIndex := 0
		if strings.HasPrefix(mine.mnemonic, "csrr") {
			nameIndex = 1
		}
		if len(operands) > nameIndex {
			switch operands[nameIndex] {
			case "fflags", "frm", "fcsr":
				fpAliases := map[string]bool{
					"frflags": true, "fsflags": true, "fsflagsi": true,
					"frrm": true, "fsrm": true, "fsrmi": true,
					"frcsr": true, "fscsr": true,
				}
				if fpAliases[real.mnemonic] {
					// The remaining operands (rd if present, and
					// rs1 or the immediate) should still agree once
					// the CSR name is removed -- except that when rd
					// is discarded, objdump's alias still names it
					// "zero" for the immediate forms (fsflagsi,
					// fsrmi) but not the register ones (fsflags,
					// fsrm).
					rest := append(operands[:nameIndex:nameIndex], operands[nameIndex+1:]...)
					if nameIndex == 0 && strings.HasSuffix(mine.mnemonic, "i") {
						rest = append([]string{"zero"}, rest...)
					}
					if strings.Join(rest, ",") == real.operands {
						return "fp CSR alias", true
					}
				}
			}
		}
	}

	// Also exception 3 (an empty fence flag set printed as "unknown"):
	// objdump's binutils doesn't recognize the Zihintpause encoding of
	// "pause" as a named instruction at all, and falls back to the
	// generic fence decoding, where the empty successor set shows up
	// as the same "unknown".
	if mine.mnemonic == "pause" && real.mnemonic == "fence" && real.operands == "w,unknown" {
		return "unknown fence flags", true
	}

	// Exception 2 & 3: same mnemonic, operands line up positionally
	// except for an unnamed CSR (printed as "0x..." here, by name in
	// objdump) or an empty fence flag set (printed as "0" here,
	// "unknown" in objdump).
	if mine.mnemonic == real.mnemonic {
		mineOperands, realOperands := mine.operandList(), real.operandList()
		if len(mineOperands) == len(realOperands) {
			category := ""
			match := true
			for i := range mineOperands {
				if mineOperands[i] == realOperands[i] {
					continue
				}
				if strings.HasPrefix(mine.mnemonic, "csr") &&
					strings.HasPrefix(mineOperands[i], "0x") {
					category = "unnamed CSR"
					continue
				}
				if mine.mnemonic == "fence" &&
					mineOperands[i] == "0" && realOperands[i] == "unknown" {
					category = "unknown fence flags"
					continue
				}
				match = false
				break
			}
			if match && category != "" {
				return category, true
			}
		}
	}

	return "", false
}

// A located RISC-V binutils install.
type toolchainT struct {
	as      string
	objcopy string
	objdump string
}

// findToolchain looks for a RISC-V binutils install, first on PATH
// (under either of the usual target-triple prefixes) and then in the
// common manual-install location.
func findToolchain() (toolchainT, error) {
	prefixes := []string{"riscv64-unknown-elf-", "riscv64-linux-gnu-"}
	dirs := append([]string{""}, "/opt/riscv/bin/")

	for _, dir := range dirs {
		for _, prefix := range prefixes {
			as := findTool(dir, prefix, "as")
			objcopy := findTool(dir, prefix, "objcopy")
			objdump := findTool(dir, prefix, "objdump")
			if as != "" && objcopy != "" && objdump != "" {
				return toolchainT{as, objcopy, objdump}, nil
			}
		}
	}
	return toolchainT{}, fmt.Errorf(
		"no RISC-V binutils install found (looked for *-as/-objcopy/-objdump" +
			" on PATH and in /opt/riscv/bin); this check needs one to compare against")
}

func findTool(dir, prefix, name string) string {
	path := dir + prefix + name
	if dir == "" {
		resolved, err := exec.LookPath(path)
		if err != nil {
			return ""
		}
		return resolved
	}
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		return path
	}
	return ""
}

// objdumpDisassemble assembles the given words and disassembles the
// result with the real objdump, returning one decodedT per word.
func objdumpDisassemble(tools toolchainT, insts []uint32) ([]decodedT, error) {
	dir, err := os.MkdirTemp("", "disasmcheck")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	asmPath := dir + "/test.s"
	objPath := dir + "/test.o"
	binPath := dir + "/test.bin"

	var asm strings.Builder
	for _, inst := range insts {
		fmt.Fprintf(&asm, "\t.word 0x%08x\n", inst)
	}
	if err := os.WriteFile(asmPath, []byte(asm.String()), 0644); err != nil {
		return nil, err
	}

	// zicsr/zifencei/zihintpause cover the CSR, fence.i and pause
	// encodings; the base rv64im covers the rest.
	run := func(name string, args ...string) error {
		cmd := exec.Command(name, args...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("%s %s: %w\n%s", name, strings.Join(args, " "), err, out)
		}
		return nil
	}
	if err := run(tools.as, "-march=rv64imzicsr_zifencei_zihintpause", "-o", objPath, asmPath); err != nil {
		return nil, err
	}
	if err := run(tools.objcopy, "-O", "binary", objPath, binPath); err != nil {
		return nil, err
	}

	cmd := exec.Command(tools.objdump, "-D", "-b", "binary", "-m", "riscv:rv64", "-EL", binPath)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("objdump: %w", err)
	}

	// Lines look like "   0:\t01040283          \tlb\tt0,16(s0)".
	lineRe := regexp.MustCompile(`^\s*[0-9a-f]+:\t[0-9a-f]{8}\s*\t(.*)$`)
	// objdump uses the ABI name "s0" where disassemble.go, following
	// gdbRegNames, uses "fp"; normalize so the comparison isn't just
	// noting that difference over and over.
	fpRe := regexp.MustCompile(`\bs0\b`)

	var result []decodedT
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		m := lineRe.FindStringSubmatch(scanner.Text())
		if m == nil {
			continue
		}
		text := m[1]
		// objdump appends "# 0x..." to some instructions, guessing at
		// a computed constant or address; that's not part of the
		// instruction's text and disassemble.go doesn't produce it.
		if i := strings.Index(text, "#"); i >= 0 {
			text = text[:i]
		}
		text = fpRe.ReplaceAllString(text, "fp")
		text = strings.Join(strings.Fields(text), " ")
		result = append(result, decode(0, text))
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return result, nil
}
