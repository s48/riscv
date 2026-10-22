// Copyright 2026 Richard Kelsey. All rights reserved.
// See file LICENSE for notices and license.

// This is a very minimal implementation of a GDB server, but it works.
//
// The GDB remote serial protocol is described here, among other places.
// https://sourceware.org/gdb/current/onlinedocs/gdb.html/Remote-Protocol.html

package emulator

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
)

func GdbServer(cpu *CpuT) {
	listener, err := net.Listen("tcp", ":50001")
	if err != nil {
		panic(fmt.Sprintf("net.Listen failed: %s", err))
	}

	fmt.Printf("listening at %s\n", listener.Addr())
	for {
		conn, err := listener.Accept()
		if err != nil {
			panic(fmt.Sprintf("listener.Accept failed: %s", err))
		}
		handleConnection(conn, cpu)
	}
}

type gdbClientT struct {
	scan         *bufio.Scanner
	writer       *bufio.Writer
	cpu          *CpuT
	previousSend string // To print an error if we get a NACK.
}

// Commands are a single character, often with additional
// arguments or subcommands.
type gdbCommandHandlerT = func(client *gdbClientT, command string)

var gdbCommands = map[byte]gdbCommandHandlerT{}

func addGdbCommand(initial byte, handler gdbCommandHandlerT) {
	gdbCommands[initial] = handler
}

func handleConnection(conn net.Conn, cpu *CpuT) {
	fmt.Printf("connected\n")
	scanner := bufio.NewScanner(conn)
	writer := bufio.NewWriter(conn)
	client := &gdbClientT{scanner, writer, cpu, ""}
	scanner.Split(gdbSplit)
	for client.cpu != nil && scanner.Scan() {
		command := scanner.Text()
		switch command[0] {
		case '+':
			// do nothing
		case '-':
			panic(fmt.Sprintf("Received a NACK ('-') from GDB after '%s'",
				client.previousSend))
		case '$':
			//fmt.Printf("Received: '%s'\n", command)
			writer.WriteString("+")
			writer.Flush()
			handler := gdbCommands[command[1]]
			if handler == nil {
				panic(fmt.Sprintf("unknown GDB command '%s'\n", command))
			}
			handler(client, command[2:])
		default:
			panic(fmt.Sprintf("Received badly-formed packet from GDB: '%s'",
				command))
		}
	}
}

// '$' command '#' checksum
// The checksum is the one-byte sum of the bytes in the command,
// written in hex.  I think it is always two characters.
//
//  '+' is a positive ack, '-' a negative one.
//
// The code below assumes a reliable connection.
//
// 7D (ASCII ‘}’) is used as an escape character.  The character
// after 7D is xor-ed with 0x20, so an actual 7D is sent as 7D 5D.
// ‘#’, ‘$’ and ‘}’ must always be escaped, as well as ‘*’ when
// sent in responses.  This has not been implemented here.
//
// Responses can be run-length encoded. N copies of C are sent
// as three characters: C '*' N+29.  If 3 <= N < 98 then N+98
// is a printable ASCII character.  We don't do this.

func gdbSplit(data []byte, atEOF bool) (int, []byte, error) {
	// fmt.Printf("bytes '%s'\n", string(data))
	if len(data) < 1 {
		return 0, nil, nil
	}
	if data[0] == '+' || data[0] == '-' {
		return 1, data[:1], nil
	}
	if len(data) < 4 {
		return 0, nil, nil
	}
	if data[0] != '$' {
		return 0, nil, errors.New("no initial '$'")
	}
	checksum := byte(0)
	for i, b := range data[1:] {
		if b == '#' {
			if len(data) < i+2 {
				return 0, nil, nil // checksum hasn't arrived
			}
			sum, err := strconv.ParseUint(string(data[i+2:i+4]), 16, 8)
			if err != nil {
				panic(fmt.Sprintf("Failed to parse checksum '%s': %s",
					data[i+2:i+4], err))
			} else if sum != uint64(checksum) {
				panic(fmt.Sprintf("Wrong checksum: us %02X them %02X",
					checksum, sum))
			} else {
				return i + 4, data[:i+1], nil
			}
		}
		checksum += b
	}
	return 0, nil, nil
}

func (client *gdbClientT) send(payload string) {
	client.previousSend = makePacket(payload)
	client.writer.WriteString(client.previousSend)
	client.writer.Flush()
}

func makePacket(payload string) string {
	checksum := byte(0)
	for _, b := range []byte(payload) {
		checksum += b
	}
	message := fmt.Sprintf("$%s#%02x", payload, checksum)
	//fmt.Printf("Sending '%s'\n", message)
	return message
}

//----------------------------------------------------------------
// Commands

func init() {
	addGdbCommand('?', (*gdbClientT).sendHaltReasonCommand)
	addGdbCommand('g', (*gdbClientT).sendRegistersCommand)
	addGdbCommand('H', (*gdbClientT).threadCommand)
	addGdbCommand('m', (*gdbClientT).readMemoryCommand)
	addGdbCommand('q', (*gdbClientT).queryCommand)
	addGdbCommand('Q', (*gdbClientT).setCommand)
	addGdbCommand('v', (*gdbClientT).vCommand)
	addGdbCommand('z', (*gdbClientT).removeBreakpointCommand)
	addGdbCommand('Z', (*gdbClientT).addBreakpointCommand)
}

// Called once at the start of a connection.
func (client *gdbClientT) sendHaltReasonCommand(command string) {
	// 05 is the trap signal, required if 'thread' is used.
	// 2 is the ID of the stopped thread.
	client.send("T05;thread:2;")
}

// <address>,<number of bytes>
func (client *gdbClientT) readMemoryCommand(command string) {
	args := strings.Split(command, ",")
	address, _ := strconv.ParseUint(args[0], 16, 64)
	count, _ := strconv.ParseUint(args[1], 16, 64)
	buffer := &bytes.Buffer{}
	for i := range count {
		fmt.Fprintf(buffer, "%02x", client.cpu.mustReadByte(uint64(address+i)))
	}
	client.send(buffer.String())
}

func (client *gdbClientT) sendRegistersCommand(command string) {
	buffer := &bytes.Buffer{}
	bytes := make([]byte, 8)
	for _, reg := range client.cpu.Registers {
		binary.BigEndian.PutUint64(bytes, reg)
		n := binary.LittleEndian.Uint64(bytes)
		fmt.Fprintf(buffer, "%016x", n)
	}
	binary.BigEndian.PutUint64(bytes, client.cpu.Pc)
	n := binary.LittleEndian.Uint64(bytes)
	fmt.Fprintf(buffer, "%016x", n)
	client.send(buffer.String())
}

// What we support:
//  PacketSize=1000   largest packet, picked at random
//  vContSupported    vCont commands
//
// What we claim to support but don't yet:
//  swbreak           report the ‘swbreak’ stop reason for memory breakpoints
//  hwbreak           report the ‘hwbreak’ stop reason for hardware breakpoints
//  exec-file:read    understand the ‘qXfer:exec-file:read’ packet
//  no-resumed        report the ‘N’ stop reply if there are no runnable threads

func (client *gdbClientT) queryCommand(command string) {
	if strings.HasPrefix(command, "Supported") {
		client.send("PacketSize=1000;vContSupported+;swbreak+;hwbreak+;exec-file:read+;no-resumed+")
	} else if command == "TStatus" {
		client.send("T0;tnotrun:0")
	} else if command == "TfV" {
		client.send("")
	} else if command == "fThreadInfo" {
		// Our one thread ID.
		client.send("mp1.2")
	} else if command == "sThreadInfo" {
		// End of thread list.
		client.send("l")
	} else if command == "Attached" {
		client.send("0")
	} else if command == "Offsets" {
		client.send("")
	} else if command == "Symbol::" {
		client.send("OK")
	} else if command == "TfP" {
		client.send("l")
	} else {
		fmt.Printf("unknown query command 'q%s'\n", command)
	}
}

func (client *gdbClientT) setCommand(command string) {
	varAndValue := strings.Split(command, ":")
	value := varAndValue[1]
	switch varAndValue[0] {
	case "ThreadEvents":
		if value == "0" {
			panic("ThreadEvents turned off.")
		}
	default:
		panic(fmt.Sprintf("Unknown set command '%s'", command))
	}
}

func (client *gdbClientT) vCommand(command string) {
	if command == "Cont?" {
		client.send("Cont;c;C;t;s;S")
	} else if command == "MustReplyEmpty" {
		client.send("")
	} else if strings.HasPrefix(command, "Cont;") {
		client.vContCommand(strings.TrimPrefix(command, "Cont;"))
	} else if command == "Kill;1" {
		client.send("")
		client.cpu = nil // close the connection
	} else {
		fmt.Printf("unknown command 'v%s'\n", command)
	}
}

func (client *gdbClientT) threadCommand(command string) {
	if command == "g0" {
		// The number is the thread ID for subsequent commands.
		// 0 means pick any thread.
		client.send("OK")
	} else if command == "c-1" {
		// Continue all threads.  gdbserver reports an error, probably
		// because it supports vCont, as do we.
		client.send("E01")
	} else {
		fmt.Printf("unknown command 'H%s'\n", command)
	}
}

// vCont[;action[:thread-id]]...
//
// The caller strips off "vCont;".  The command we receive for a
// 'step' command is "vCont;s:2;c".  Our thread ID is 2, so this tells
// us to step that thread.  The final 'c' is confusing.  The doc says
// it applies to all threads, but it must mean that it applies to all
// threads that have not been previously referenced.

func (client *gdbClientT) vContCommand(command string) {
	commands := strings.Split(command, ";")
	for i, command := range commands {
		args := strings.Split(command, ":")
		if len(args) == 2 && args[1] == "2" {
			switch args[0] {
			case "s":
				client.cpu.runOneInstruction()
				client.send("T05;thread:2;")
				return
			default:
				panic(fmt.Sprintf("Unrecognized vCont action '%s' in '%s'", args[0], command))
			}
		} else if len(args) == 1 && i == len(commands)-1 {
			if command[0] == 'c' {
				client.cpu.Run()
				if client.cpu.Status == cpuBreakpoint {
					client.send("T05;thread:2;swbreak:;")
				} else if client.cpu.Status == cpuExited {
					client.send(fmt.Sprintf("W%02X", client.cpu.ExitStatus))
				}
				return
			}
		}
	}
	panic("Received a vCont with no action for our thread: '" + command + "'")
}

// $Z0,8000020c,4

func (client *gdbClientT) addBreakpointCommand(command string) {
	fields := strings.Split(command, ",")
	address, err := strconv.ParseUint(fields[1], 16, 64)
	if err != nil {
		panic(fmt.Sprintf("Unable to parse breakpoint address '%s'", fields[1]))
	}
	switch fields[0] {
	case "0":
		client.cpu.addBreakpoint(address)
		client.send("OK")
	default:
		panic(fmt.Sprintf("Unknown breakpoint command 'Z%s'", command))
	}
}

func (client *gdbClientT) removeBreakpointCommand(command string) {
	fields := strings.Split(command, ",")
	address, err := strconv.ParseUint(fields[1], 16, 64)
	if err != nil {
		panic(fmt.Sprintf("Unable to parse breakpoint address '%s'", fields[1]))
	}
	switch fields[0] {
	case "0":
		client.cpu.removeBreakpoint(address)
		client.send("OK")
	default:
		panic(fmt.Sprintf("Unknown remove breakpoint command 'z%s'", command))
	}
}
