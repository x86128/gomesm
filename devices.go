package main

import "log"

// MemRegion of device attached to bus
type MemRegion struct {
	start uint16
	end   uint16
}

// Device interface
type Device interface {
	reset()
	getName() string
	read(addr uint16) besmWord
	write(addr uint16, value besmWord)
}

// Memory holds read/write data
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

// Bus is used by CPU to read/write mmaped devices
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
	if addr == 0 {
		return 0
	}
	for i, mmap := range bus.mmaps {
		if mmap.start <= addr && addr <= mmap.end {
			return bus.devices[i].read(addr - mmap.start)
		}
	}
	log.Printf("BUS: %s read out of bounds: 0o%o", bus.name, addr)
	return 0o7654123450517667 // garbage from unconnected bus
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
