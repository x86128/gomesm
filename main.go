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

type Cpu struct {
	PC      uint16
	ACC     besmWord
	M       [16]uint16
	ibus    *Bus
	dbus    *Bus
	right   bool
	irCache besmWord
	ir      besmWord
}

func (cpu *Cpu) reset() {
	cpu.PC = 1
	cpu.ACC = 0
	cpu.M = [16]uint16{0}
}

func (cpu *Cpu) step() {
	cpu.ir = cpu.irCache & 0o77777777
	pcNext := cpu.PC
	if !cpu.right {
		cpu.irCache = cpu.ibus.read(cpu.PC)
		cpu.ir = cpu.irCache >> 24
	} else {
		pcNext = (cpu.PC + 1) & 0o77777
	}
	cpu.right = !cpu.right
	cpu.PC = pcNext
}

func (cpu *Cpu) state() {
	fmt.Printf("PC:\t%05o right:%t IR:%016o\n", cpu.PC, cpu.right, cpu.ir)
	fmt.Printf("M:\t%05v\n", cpu.M)
	fmt.Printf("ACC:\t%016o\n", cpu.ACC)
}

func newCPU(ibus *Bus, dbus *Bus) *Cpu {
	cpu := Cpu{}
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

	cpu := newCPU(ibus, dbus)
	rom.write(1, 0o6666666677777777)
	cpu.step()
	cpu.state()
	fmt.Println("===")
	cpu.step()
	cpu.state()
}
