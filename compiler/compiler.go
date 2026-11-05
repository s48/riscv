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
	cps.AllocateRegisters(entryPoint, intRegisters)
	if debug.Contains("register") {
		fmt.Printf("After registers allocation ================")
		cps.PpCps(entryPoint)
	}

	codeBlocks := cps.Assemble(entryPoint, &codeGeneratorT{})
	instCount := 0
	for _, block := range codeBlocks {
		for _, insts := range block.Instructions {
			instCount += len(insts)
		}
	}
	code := make([]byte, instCount*4)
	lineData := make([]emulator.LineRecord, instCount)
	i := 0
	labels := map[uint64]string{}
	for _, block := range codeBlocks {
		labels[uint64(block.Offset())*4] = block.String()
	}

	for _, block := range codeBlocks {
		if *printListing {
			fmt.Printf("%s:\n", block)
		}
		for _, insts := range block.Instructions {
			for _, inst := range insts {
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

	inputs := make([]int, len(entryPoint.Outputs)-1)
	for i, vart := range entryPoint.Outputs[1:] {
		if vart.Register != nil {
			inputs[i] = vart.Register.(*registerT).number
		}
	}
	return cpu, inputs, instCount
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
