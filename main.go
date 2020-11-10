package main

import (
	"fmt"
	"log"
)

type MemRegion struct {
	start uint16
	end   uint16
}

type Device interface {
	reset()
	getName() string
	read(addr uint16) uint64
	write(addr uint16, value uint64)
}

type Memory struct {
	name string
	size uint16
	data []uint64
}

func (m *Memory) reset() {
	m.data = make([]uint64, m.size)
}

func (m *Memory) getName() string {
	return m.name
}

func (m *Memory) read(addr uint16) uint64 {
	if addr < m.size {
		return m.data[addr]
	}

	log.Printf("MEM: Read from %s out of bounds: 0o%o", m.name, addr)
	return 0xDEADBEEF
}

func (m *Memory) write(addr uint16, value uint64) {
	if addr < m.size {
		m.data[addr] = value
	} else {
		log.Printf("MEM: Write to %s out of bounds: 0o%o", m.name, addr)
	}
}

func newMemory(name string, size uint16) Memory {
	return Memory{name, size, make([]uint64, size)}
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

func (bus *Bus) read(addr uint16) uint64 {
	for i, mmap := range bus.mmaps {
		if mmap.start <= addr && addr <= mmap.end {
			return bus.devices[i].read(addr - mmap.start)
		}
	}
	log.Printf("BUS: %s read out of bounds: 0o%o", bus.name, addr)
	return 0xDEADBEEF
}

func (bus *Bus) write(addr uint16, value uint64) {
	for i, mmap := range bus.mmaps {
		if mmap.start <= addr && addr <= mmap.end {
			bus.devices[i].write(addr-mmap.start, value)
			return
		}
	}
	log.Printf("BUS: %s write out of device address space,  0o%o", bus.name, addr)
}

type besmWord uint64

type Cpu struct {
	PC   uint16
	ACC  besmWord
	M    [16]uint16
	ibus *Bus
	dbus *Bus
}

func (cpu *Cpu) reset() {
	cpu.PC = 1
	cpu.ACC = 0
	cpu.M = [16]uint16{0}
}

func (cpu *Cpu) step() {
	cpu.PC++
}

func (cpu *Cpu) state() {
	fmt.Printf("PC:\t%05o\n", cpu.PC)
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
	cpu.step()
	cpu.state()
}
