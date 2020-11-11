package main

import (
	"fmt"
	"log"
)

type besmWord uint64

type MemRegion struct {
	start uint16
	end   uint16
}

type Device interface {
	reset()
	getName() string
	read(addr uint16) besmWord
	write(addr uint16, value besmWord)
}

type Memory struct {
	name string
	size uint16
	data []besmWord
}

func (m *Memory) reset() {
	m.data = make([]besmWord, m.size)
}

func (m *Memory) getName() string {
	return m.name
}

func (m *Memory) read(addr uint16) besmWord {
	if addr < m.size {
		return m.data[addr]
	}

	log.Printf("MEM: Read from %s out of bounds: 0o%o", m.name, addr)
	return 0xDEADBEEF
}

func (m *Memory) write(addr uint16, value besmWord) {
	if addr < m.size {
		m.data[addr] = value
	} else {
		log.Printf("MEM: Write to %s out of bounds: 0o%o", m.name, addr)
	}
}

func newMemory(name string, size uint16) Memory {
	return Memory{name, size, make([]besmWord, size)}
}

type Bus struct {
	name    string
	mmaps   []MemRegion
	devices []Device
}

func newBus(name string) *Bus {
	return &Bus{name, nil, nil}
}

func (bus *Bus) reset() {
	for _, dev := range bus.devices {
		dev.reset()
	}
}

func (bus *Bus) attach(memRegion MemRegion, dev Device) {
	for i, mmap := range bus.mmaps {
		if mmap.start <= memRegion.start && memRegion.end <= mmap.end {
			log.Println("BUS: Device", dev.getName(), "clamps with", bus.devices[i].getName())
			return
		}
	}
	bus.mmaps = append(bus.mmaps, memRegion)
	bus.devices = append(bus.devices, dev)
}

func (bus *Bus) read(addr uint16) besmWord {
	for i, mmap := range bus.mmaps {
		if mmap.start <= addr && addr <= mmap.end {
			return bus.devices[i].read(addr - mmap.start)
		}
	}
	log.Printf("BUS: %s read out of bounds: 0o%o", bus.name, addr)
	return 0xDEADBEEF
}

func (bus *Bus) write(addr uint16, value besmWord) {
	for i, mmap := range bus.mmaps {
		if mmap.start <= addr && addr <= mmap.end {
			bus.devices[i].write(addr-mmap.start, value)
			return
		}
	}
	log.Printf("BUS: %s write out of device address space,  0o%o", bus.name, addr)
}

// CPU state
type CPU struct {
	PC      uint16     // instruction pointer
	ACC     besmWord   // accumulator
	M       [16]uint16 // M registers (index registers, address modifiers)
	ibus    *Bus       // instruction bus
	dbus    *Bus       // data bus
	right   bool       // right or left instruction flag
	irCache besmWord   // instruction cache register
	ir      besmWord   // current being executed instruction
	stack   bool       // current instruction in stack mode
	uaddr   uint16     // u_addr = i_addr + m[ind] + c_addr (if set)
}

func (cpu *CPU) reset() {
	cpu.PC = 1
	cpu.ACC = 0
	cpu.M = [16]uint16{0}
	cpu.right = false
}

func (cpu *CPU) atx() {
	cpu.dbus.write(cpu.uaddr, cpu.ACC)
	if cpu.stack {
		cpu.M[15]++
	}
}

func (cpu *CPU) step() {
	// FETCH instruction from cache or
	cpu.ir = cpu.irCache & 0o77777777
	pcNext := cpu.PC
	// if last step was executed right instruction
	if !cpu.right {
		// fetch new instruction from insruction bus
		cpu.irCache = cpu.ibus.read(cpu.PC)
		cpu.ir = cpu.irCache >> 24
	} else {
		pcNext = (cpu.PC + 1) & 0o77777
	}
	cpu.right = !cpu.right
	// DECODE instruction
	//ind := (cpu.ir >> 20) & 0xF
	//fmt.Println("ind:", ind)
	// advance instrunction pointer
	cpu.PC = pcNext
}

func (cpu *CPU) state() {
	fmt.Printf("PC:\t%05o right:%t IR:%016o\n", cpu.PC, cpu.right, cpu.ir)
	fmt.Printf("M:\t%05v\n", cpu.M)
	fmt.Printf("ACC:\t%016o\n", cpu.ACC)
}

// Short address opcodes (BIT20 = 0)
const (
	OpATX = iota
	OpSTX
	OpMOD
	OpXTS
	OpADD
	OpSUB
	OpRSUB
	OpAMX
	OpXTA
	OpAAX
	OpAEX
	OpARX
	OpAVX
	OpAOX
	OpDIV
	OpMUL
	OpAPX
	OpAUX
	OpACX
	OpANX
	OpEADDX
	OpESUBX
	OpASX
	OpXTR
	OpRTE
	OpYTA
	OpE32
	OpE33
	OpEADDN
	OpESUB
	OpASN
	OpNTR
	OpATI
	OpSTI
	OpITA
	OpITS
	OpMTJ
	OpJADDM
	OpE46
	OpE47
	OpE50
	OpE51
	OpE52
	OpE53
	OpE54
	OpE55
	OpE56
	OpE57
	OpE60
	OpE61
	OpE62
	OpE63
	OpE64
	OpE65
	OpE66
	OpE67
	OpE70
	OpE71
	OpE72
	OpE73
	OpE74
	OpE75
	OpE76
	OpE77
)

// Long address opcodes (BIT20 = 1)
const (
	OpE20 = 0o200 + iota*0o10
	OpE21
	OpUTC
	OpWTC
	OpVTM
	OpUTM
	OpUZA
	OpUIA
	OpUJ
	OpVJM
	OpIJ
	OpSTOP
	OpVZM
	OpVIM
	OpE36
	OpVLM
)

func emitOp(ind uint16, op uint16, addr uint16) (word besmWord, err error) {
	word = besmWord(ind&0xF) << 20
	addr = addr & 0o77777
	if op <= OpE77 {
		if addr >= 0o7777 && addr <= 0o67777 {
			return 0, fmt.Errorf("Address: %05o out of range for `short` command", addr)
		}
		word |= (besmWord(op) << 12) + (besmWord(addr) & 0o7777)
		if addr >= 0o70000 {
			word |= 0o1000000
		}
	} else {
		word |= (besmWord(op) << 12) + (besmWord(addr) & 0o77777)
	}
	return word, nil
}

func newCPU(ibus *Bus, dbus *Bus) *CPU {
	cpu := CPU{}
	cpu.ibus = ibus
	cpu.dbus = dbus
	cpu.reset()
	return &cpu
}

func main() {
	rom := newMemory("ROM", 1024)
	ram := newMemory("RAM", 1024)

	ibus := newBus("IBUS")
	dbus := newBus("DBUS")
	ibus.attach(MemRegion{0, 2047}, &rom)
	dbus.attach(MemRegion{0, 1023}, &ram)

	// cpu := newCPU(ibus, dbus)

	fmt.Printf("%0o\n", OpE77)
}
