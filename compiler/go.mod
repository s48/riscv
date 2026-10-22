module github.com/s48/riscv/compiler

go 1.27.1

require github.com/s48/transform v0.1.20
require github.com/s48/riscv/emulator v0.0.0

require (
	golang.org/x/mod v0.22.0 // indirect
	golang.org/x/sync v0.9.0 // indirect
	golang.org/x/tools v0.27.0 // indirect
)

replace github.com/s48/transform => ../../transform
replace github.com/s48/riscv/emulator => ../emulator
