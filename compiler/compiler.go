// Copyright 2026 Richard Kelsey. All rights reserved.
// See file LICENSE for notices and license.

package compiler

import (
	"encoding/binary"
	"flag"
	"fmt"
	"math/big"
	"strings"

	"github.com/s48/riscv/emulator"
	"github.com/s48/transform/cps"
	"github.com/s48/transform/front"
	"github.com/s48/transform/util"
)

var printListing = flag.Bool("listing", false, "Print listing.")

func CompileModule(
	sourceDir string,
	entryPointName string,
	objectFileName string,
) (*emulator.CpuT, []int, int) {
	debug := parseDebugPhases()
	frontEnd := front.MakeFrontEnd([]string{"github.com/s48/riscv/compiler/primitives"}, nil)
	frontEnd.LoadPackage(sourceDir)
	frontEnd.ParseAndTypeCheck()
	entryPackage := frontEnd.Packages[0]

	cps.Lambdas = util.NewSet[*cps.CallNodeT]()
	cps.NextVariableId = 0

	frontEnd.ProcessDecls()

	var entryVar *cps.VariableT
	for vart, decl := range frontEnd.Funcs {
		def := frontEnd.TypesInfo.Defs[decl.Name]
		if def.Pkg() == entryPackage.TypesPackage && def.Name() == entryPointName {
			entryVar = vart
			break
		}
	}
	if entryVar == nil {
		panicf("entry point '%s' not found", entryPointName)
	}

	funcVars, funcVals := frontEnd.ConvertFuncs(entryVar, debug.Contains("convert"))

	for lambda := range cps.Lambdas {
		convertTypes(lambda)
	}

	for i, val := range funcVals {
		front.SimplifyTopLevel(val)
		if debug.Contains("simplify") {
			fmt.Printf("%s simplified ================", funcVars[i].Name)
			cps.PpCps(val)
		}
	}

	entryPoint := funcVals[0]
	putFuncsInLetrec(entryPoint, funcVars[1:], funcVals[1:])

	// Returns from entryPoint become terminate-with-value syscalls.
	syscall := cps.LookupPrimop("syscall")
	for _, ref := range entryPoint.Outputs[0].Refs {
		call := ref.Parent()
		call.Primop = syscall
		value := cps.DetachInput(call.Inputs[1])
		cps.Push(&call.Inputs, nil)
		cps.AttachInput(call, 2, value)
		// 93 is terminate with value.
		cps.AttachInput(call, 1, cps.MakeLiteral(big.NewInt(93), theUint64Type))
	}

	if debug.Contains("preregister") {
		fmt.Printf("Before register allocation ================")
		cps.PpCps(entryPoint)
	}
	allocation := cps.AllocateRegisters(entryPoint, intRegisters)
	if debug.Contains("register") {
		fmt.Printf("After registers allocation ================")
		cps.PpCps(entryPoint)
	}

	// The entry point comes first.  Its code starts at memoryStart.
	generator := makeCodeGenerator(allocation)
	codeBlocks := []*cps.CodeBlockT[instT]{}
	blockOffsets := []int{}                 // block offsets are relative to their procedure
	procOffsets := map[*cps.CallNodeT]int{} // in instructions
	instCount := 0
	for _, proc := range cps.FindProcs(entryPoint) {
		procOffsets[proc] = instCount
		generator.setProc(proc, proc == entryPoint)
		for _, block := range cps.Assemble(proc, generator) {
			cps.Push(&codeBlocks, block)
			cps.Push(&blockOffsets, instCount+block.Offset())
		}
		instCount = blockOffsets[len(blockOffsets)-1]
		last := codeBlocks[len(codeBlocks)-1]
		for _, insts := range last.Instructions {
			instCount += len(insts)
		}
	}

	// The heap prelude, if needed, goes first, so everything else moves up.
	prelude := []uint32{}
	if generator.usesHeap {
		prelude = heapPrelude()
		for i := range blockOffsets {
			blockOffsets[i] += len(prelude)
		}
		for proc := range procOffsets {
			procOffsets[proc] += len(prelude)
		}
		instCount += len(prelude)
	}

	code := make([]byte, instCount*4)
	lineData := make([]emulator.LineRecord, instCount)
	labels := map[uint64]string{}
	if 0 < len(prelude) {
		labels[0] = "prelude"
	}
	for i, block := range codeBlocks {
		labels[uint64(blockOffsets[i])*4] = block.String()
	}

	if 0 < len(prelude) && *printListing {
		fmt.Printf("prelude:\n")
	}
	for i, bits := range prelude {
		binary.LittleEndian.PutUint32(code[i*4:], bits)
		if *printListing {
			fmt.Printf("  %s\n", emulator.Disassemble(uint64(i*4), bits, labels))
		}
	}

	for k, block := range codeBlocks {
		if *printListing {
			fmt.Printf("%s:\n", block)
		}
		i := blockOffsets[k] * 4
		for _, insts := range block.Instructions {
			for _, inst := range insts {
				if inst.callTo != nil {
					// Fill in the offset to the called procedure.
					jal := &riscvInstsT{}
					jal.jFormat(procOffsets[inst.callTo]*4-i, rReturn, 0x6f)
					inst.bits = jal.instructions[0]
				}
				binary.LittleEndian.PutUint32(code[i:], inst.bits)
				if inst.pos != 0 {
					position := frontEnd.FileSet.Position(inst.pos)
					lineData[i/4].Line = position.Line
					lineData[i/4].Filename = position.Filename
				}
				if *printListing {
					fmt.Printf("  %-20s %d\n",
						emulator.Disassemble(uint64(i), inst.bits, labels),
						inst.pos)
				}
				i += 4
			}
		}
	}

	memoryStart := uint64(emulator.MemoryStart)
	if objectFileName != "" {
		emulator.WriteElfProgram(code, memoryStart, memoryStart, lineData, objectFileName)
	}

	cpu := emulator.NewCpu()
	cpu.Pc = memoryStart
	cpu.CopyToMemory(memoryStart, code)

	// The inputs are passed in a0-a7.
	inputs := make([]int, len(entryPoint.Outputs)-1)
	for i := range inputs {
		inputs[i] = 10 + i
	}
	return cpu, inputs, instCount
}

// Initializes the thread object, at MemoryStart+0x50000, and the heap,
// which is the rest of that 64KB block.  This uses only rThread and
// rTemp, so the entry point's arguments in a0-a7 are untouched.  The
// prelude is at the start of the code, so auipc gives MemoryStart plus
// the offset.  (lui would sign-extend 0x80050000.)
func heapPrelude() []uint32 {
	insts := &riscvInstsT{}
	insts.auipc(0x50, rThread)
	insts.addImm(rThread, rTemp, 16) // heap starts after the thread object
	insts.store(rTemp, rThread, threadHeapPointer)
	insts.lui(0x10, rTemp)                           // 64KB
	insts.rFormat(0, rThread, rTemp, 0, rTemp, 0x33) // add
	insts.store(rTemp, rThread, threadHeapLimit)
	return insts.instructions
}

func putFuncsInLetrec(
	entryPoint *cps.CallNodeT,
	funcVars []*cps.VariableT,
	funcVals []*cps.CallNodeT,
) {
	vars := []*cps.VariableT{}
	vals := []cps.NodeT{}
	for i, vart := range funcVars {
		val := funcVals[i]
		switch len(vart.Refs) {
		case 0:
			cps.Erase(val)
		case 1:
			cps.InlineProcedure(vart.Refs[0].Parent(), val)
		default:
			delete(vart.Flags, "package")
			vars = append(vars, vart)
			vals = append(vals, val)
		}
	}

	if 0 < len(vars) {
		letrec := cps.MakeCall(cps.LookupPrimop("letrec"), vars, vals...)
		cps.AttachNext(letrec, cps.DetachNext(entryPoint.Next[0]))
		cps.AttachNext(entryPoint, letrec)
		front.SimplifyTopLevel(entryPoint)
	}
}

//----------------------------------------------------------------

var showPhases util.ListFlagT
var phaseNames = []string{"convert", "simplify", "preregister", "register"}
var phaseFlags = map[string]string{}

func init() {
	flag.Var(&showPhases, "show", "compiler phases to display")
	for _, phase := range phaseNames {
		phaseFlags[phase] = phase
	}
	// Some aliases.
	phaseFlags["converted"] = "convert"
	phaseFlags["simplified"] = "simplify"
	phaseFlags["registers"] = "register"
}

func parseDebugPhases() util.SetT[string] {
	phases := util.NewSet[string]()
	for _, name := range showPhases.Values() {
		phase, found := phaseFlags[name]
		if !found {
			panicf("No such phase: '%s' (phases are '%s')", name, strings.Join(phaseNames, "', '"))
		}
		phases.Add(phase)
	}
	return phases
}

//----------------------------------------------------------------

func panicf(format string, args ...any) {
	panic(fmt.Sprintf(format, args...))
}
