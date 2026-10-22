package main

import (
	"debug/elf"
	"flag"
	"fmt"
	"io"

	"github.com/s48/riscv/emulator"
)

func main() {
	filename := flag.String("elf", "", "ELF file")
	debug := flag.Bool("debug", false, "run GDB server")
	flag.Parse()

	cpu := emulator.NewCpu()

	file, err := elf.Open(*filename)
	if err != nil {
		panic(fmt.Sprintf("Failed to read '%s': %s", *filename, err))
	}
	fmt.Printf("class %s data %s version %s\n", file.Class, file.Data, file.Version)
	fmt.Printf("type %s OSABI: %s machine %s\n", file.Type, file.OSABI, file.Machine)
	//fmt.Printf("entry %X\n", file.Entry)
	cpu.Pc = file.Entry
	fmt.Printf("progs:\n")
	for _, prog := range file.Progs {
		fmt.Printf("  type %s flags %s size %d address %X offset %X align %X\n",
			prog.Type, prog.Flags, prog.Filesz, prog.Vaddr, prog.Off, prog.Align)
		if prog.Type == elf.PT_LOAD {
			data, err := io.ReadAll(prog.Open())
			if err != nil {
				panic(fmt.Sprintf("Failed to get prog data: %s", err))
			}
			cpu.CopyToMemory(prog.Vaddr, data)
		}
	}
	/*
		fmt.Printf("sections:\n")
		for _, section := range file.Sections {
			fmt.Printf("  name '%s' type %s flags %s size %d address %X\n",
				section.Name, section.Type, section.Flags, section.Size, section.Addr)
		}
	*/
	/*
		for _, section := range file.Sections {
			if section.Flags & elf.SHF_ALLOC != 0 && section.Type != elf.SHT_NOBITS {
				data, err := section.Data()
				if err != nil {
					panic(fmt.Sprintf("Failed to get section data: %s", err))
				}
				cpu.CopyToMemory(section.Addr, data)
			}
		}
	*/
	//fmt.Printf("first instruction is %08X at %X\n", cpu.ReadWord(cpu.Pc), cpu.Pc)
	/*
		fmt.Printf("symbols:\n")
		symbols, err1 := file.Symbols()
		if err1 != nil {
			panic(fmt.Sprintf("Failed to read symbols: %s", err))
		}
		for _, symbol := range symbols {
			fmt.Printf("  name '%s' section %d value %X size %X\n",
				symbol.Name, symbol.Section, symbol.Value, symbol.Size)
			if len(symbol.Name) == 0 {
				fmt.Printf("  %+v\n", symbol)
			}
		}
	*/
	outElf := emulator.ConvertElf(file)
	emulator.WriteElf(outElf, "foo.bin")

	if *debug {
		emulator.GdbServer(cpu)
	} else {
		cpu.Run()
		fmt.Printf("exit status %d\n", cpu.ExitStatus)
	}
}
