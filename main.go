package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
)

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

type besmWord uint64

func emitOp(ind uint16, op uint16, addr uint16) (word besmWord, err error) {
	word = besmWord(ind&0xF) << 20
	addr = addr & 0o77777
	if op <= OpE77 {
		if addr >= 0o7777 && addr <= 0o67777 {
			return 0, fmt.Errorf("Address: %05o out of range for `short` command", addr)
		}
		word |= (besmWord(op) << 12) + (besmWord(addr) & 0o7777)
		if addr >= 0o70000 {
			word |= 0o1000000 // set address extension BIT19 = 1
		}
	} else {
		word |= (besmWord(op) << 12) + (besmWord(addr) & 0o77777)
	}
	return word, nil
}

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
	pcNext  uint16     // next PC after current instruction execution
	Acc     besmWord   // accumulator
	M       [16]uint16 // M registers (index registers, address modifiers)
	ibus    *Bus       // instruction bus
	dbus    *Bus       // data bus
	right   bool       // right or left instruction flag
	irCache besmWord   // instruction cache register
	ir      besmWord   // current being executed instruction
	stack   bool       // current instruction in stack mode
	Running bool

	irOp   uint16
	irIND  uint16
	irAddr uint16

	cActive bool
	cReg    uint16
	vAddr   uint16 // ir_addr + c_mod  if set c_active (OpUTC OpWTC)

	rrReg uint16 // machine mode and flag register
}

func (cpu *CPU) reset() {
	cpu.PC = 1
	cpu.Acc = 0
	cpu.M = [16]uint16{0}
	cpu.right = false
	cpu.Running = false
}

func (cpu *CPU) setRLog() {
	cpu.rrReg = cpu.rrReg&0b11100011 | 0b0000100
}

func (cpu *CPU) setRMul() {
	cpu.rrReg = cpu.rrReg&0b11100011 | 0b0001000
}

func (cpu *CPU) setRAdd() {
	cpu.rrReg = cpu.rrReg&0b11100011 | 0b0010000
}

func (cpu *CPU) uAddr() uint16 {
	return (cpu.M[cpu.irIND] + cpu.vAddr) & 0o77777
}

func (cpu *CPU) atx() {
	cpu.dbus.write(cpu.uAddr(), cpu.Acc)
	if cpu.stack {
		cpu.M[15] = (cpu.M[15] + 1) & 0o77777
	}
}

func (cpu *CPU) stx() {
	cpu.dbus.write(cpu.uAddr(), cpu.Acc)
	cpu.M[15] = (cpu.M[15] - 1) & 0o77777
	cpu.Acc = cpu.dbus.read(cpu.M[15])
	cpu.setRLog()
}

func (cpu *CPU) xts() {
	cpu.dbus.write(cpu.M[15], cpu.Acc)
	cpu.M[15] = (cpu.M[15] + 1) & 0o77777
	cpu.Acc = cpu.dbus.read(cpu.uAddr())
	cpu.setRLog()
}

func (cpu *CPU) aax() {
	if cpu.stack {
		cpu.M[15] = (cpu.M[15] - 1) & 0o77777
	}
	cpu.Acc = cpu.Acc & cpu.dbus.read(cpu.uAddr())
	cpu.setRLog()
}

func (cpu *CPU) aex() {
	if cpu.stack {
		cpu.M[15] = (cpu.M[15] - 1) & 0o77777
	}
	cpu.Acc = cpu.Acc ^ cpu.dbus.read(cpu.uAddr())
	cpu.setRLog()
}

func (cpu *CPU) aox() {
	if cpu.stack {
		cpu.M[15] = (cpu.M[15] - 1) & 0o77777
	}
	cpu.Acc = cpu.Acc | cpu.dbus.read(cpu.uAddr())
	cpu.setRLog()
}

func (cpu *CPU) xtr() {
	if cpu.stack {
		cpu.M[15] = (cpu.M[15] - 1) & 0o77777
	}
	// TODO: in real MESM6 r[5:0] = dbus[46:41]
	// TODO: bin r[6] is "in interrupt" flag - ignoring
	cpu.rrReg = cpu.rrReg&0b1000000 | uint16(cpu.dbus.read(cpu.uAddr())&0o77)
}

func (cpu *CPU) rte() {
	// TODO: in real MESM6 Acc[47:42] = r[5:0] (exponent = r)
	cpu.Acc = besmWord(cpu.rrReg) & 0o77
}

func (cpu *CPU) xta() {
	if cpu.stack {
		cpu.M[15] = (cpu.M[15] - 1) & 0o77777
	}
	cpu.Acc = cpu.dbus.read(cpu.uAddr())
	cpu.setRLog()
}

func (cpu *CPU) accIsZero() bool {
	if (cpu.rrReg&0b10000) != 0 && ((cpu.Acc>>40)&0x1) == 0 {
		// additive group: non-negative
		return true
	}
	if (cpu.rrReg&0b11000) == 0b01000 && ((cpu.Acc>>47)&0x1) != 0 {
		return true
	}
	if (cpu.rrReg&0b11100) == 0b0100 && cpu.Acc == 0 {
		return true
	}
	return false
}

func (cpu *CPU) uia() {
	if !cpu.accIsZero() {
		cpu.pcNext = cpu.uAddr()
	}
}

func (cpu *CPU) uza() {
	if cpu.accIsZero() {
		cpu.pcNext = cpu.uAddr()
	}
}

func (cpu *CPU) stop() {
	// check for magic stop
	// stop 12345(6) - success
	if cpu.irIND == 6 && cpu.irAddr == 0o12345 {
		log.Println("SUCCESS STOP")
	}
	cpu.Running = false
}

func (cpu *CPU) step() {
	// FETCH instruction from cache or
	cpu.ir = cpu.irCache & 0o77777777
	cpu.pcNext = cpu.PC
	// if last step was executed right instruction
	if !cpu.right {
		// fetch new instruction from insruction bus
		cpu.irCache = cpu.ibus.read(cpu.PC)
		cpu.ir = cpu.irCache >> 24
	} else {
		cpu.pcNext = (cpu.PC + 1) & 0o77777
	}
	cpu.right = !cpu.right
	// DECODE step 1. unpack instruction
	cpu.irIND = uint16((cpu.ir >> 20) & 0xF)
	if cpu.ir&0o2000000 == 0 {
		cpu.irOp = uint16((cpu.ir & 0o770000) >> 12)
		cpu.irAddr = uint16(cpu.ir & 0o7777)
		if cpu.ir&0o2000000 != 0 {
			cpu.irAddr |= 0o70000
		}
	} else {
		cpu.irOp = uint16((cpu.ir&0o1700000)>>12 + 0o200)
		cpu.irAddr = uint16(cpu.ir & 0o77777)
	}
	// DECODE step 2. modify execution address if needed
	if cpu.cActive {
		cpu.vAddr = (cpu.irAddr + cpu.cReg) & 0o77777
	} else {
		cpu.vAddr = cpu.irAddr
	}
	cpu.cActive = false
	// DECODE step 3. set stack mode flag
	cpu.stack = false
	if cpu.irIND == 15 {
		if cpu.vAddr == 0 {
			cpu.stack = true
		} else {
			if cpu.irOp == OpSTI && cpu.uAddr() == 15 {
				cpu.stack = true
			}
		}
	}
	// EXECUTE
	switch cpu.irOp {
	case OpATX:
		cpu.atx()
	case OpXTA:
		cpu.xta()
	case OpSTX:
		cpu.stx()
	case OpXTS:
		cpu.xts()
	case OpAAX:
		cpu.aax()
	case OpAEX:
		cpu.aex()
	case OpAOX:
		cpu.aox()
	case OpUIA:
		cpu.uia()
	case OpUZA:
		cpu.uza()
	case OpSTOP:
		cpu.stop()
	default:
		log.Printf("Unimplemented opcode: %03o", cpu.irOp)
		cpu.Running = false
	}
	// advance instrunction pointer
	cpu.PC = cpu.pcNext
}

func (cpu *CPU) state() {
	fmt.Printf("PC:\t%05o right: %t IR: %08o\n", cpu.PC, cpu.right, cpu.ir)
	fmt.Printf("M:\t%05o\n", cpu.M)
	fmt.Printf("Acc:\t%016o\n", cpu.Acc)
	fmt.Printf("RR: %07b\n", cpu.rrReg)
}

func (cpu *CPU) run(printState bool) {
	cpu.Running = true
	if printState {
		cpu.state()
	}
	for cpu.Running {
		cpu.step()
		if printState {
			cpu.state()
		}
	}
}

func newCPU(ibus *Bus, dbus *Bus) *CPU {
	cpu := CPU{}
	cpu.ibus = ibus
	cpu.dbus = dbus
	cpu.reset()
	return &cpu
}

func loadOct(filename string, ibus *Bus, dbus *Bus) {
	file, err := os.Open(filename)
	if err != nil {
		log.Fatal("Cannot open file:", filename)
	}
	defer file.Close()

	rd := bufio.NewScanner(file)
	for rd.Scan() {
		t := rd.Text()
		var word, leftWord, rightWord besmWord
		if strings.HasPrefix(t, "i") {
			var iaddr, lind, laddr, rind, raddr uint16
			var lopcode, ropcode string
			n, err := fmt.Sscanf(t, "i %o %o %s %o %o %s %o", &iaddr, &lind, &lopcode, &laddr, &rind, &ropcode, &raddr)
			if err != nil || n != 7 {
				log.Println("Oct file parse error at:", t)
				continue
			}
			if len(lopcode) == 2 {
				opcode, _ := strconv.ParseUint(lopcode, 8, 6)
				leftWord, _ = emitOp(lind, uint16(opcode<<3), laddr)
			} else {
				opcode, _ := strconv.ParseUint(lopcode, 8, 7)
				if opcode > 0o77 {
					laddr |= 0o70000
					opcode &= 0o77
				}
				leftWord, _ = emitOp(lind, uint16(opcode), laddr)
			}
			word = leftWord << 24

			if len(ropcode) == 2 {
				opcode, _ := strconv.ParseUint(ropcode, 8, 6)
				rightWord, _ = emitOp(rind, uint16(opcode<<3), raddr)
			} else {
				opcode, _ := strconv.ParseUint(ropcode, 8, 7)
				if opcode > 0o77 {
					raddr |= 0o70000
					opcode &= 0o77
				}
				rightWord, _ = emitOp(rind, uint16(opcode), raddr)
			}
			word |= rightWord
			ibus.write(iaddr, word)
			// log.Printf("IBUS written at %05o data: %016o", iaddr, word)
		}
		if strings.HasPrefix(t, "d") {
			var daddr, d0, d1, d2, d3 uint16
			n, err := fmt.Sscanf(t, "d %o %o %o %o %o", &daddr, &d3, &d2, &d1, &d0)
			if err != nil || n != 5 {
				log.Println("Oct parse err at:", t)
				continue
			}
			word = (besmWord(d3) << 36) | (besmWord(d2) << 24) | (besmWord(d1) << 12) | besmWord(d0)
			dbus.write(daddr, word)
			// log.Printf("DBUS written at %05o data: %016o", daddr, word)
		}
	}
}

func main() {
	rom := newMemory("ROM", 1024)
	ram := newMemory("RAM", 1024)

	ibus := newBus("IBUS")
	dbus := newBus("DBUS")
	ibus.attach(MemRegion{0, 1023}, &rom)
	dbus.attach(MemRegion{0o2000, 0o2000 + 1023}, &ram)
	loadOct("tests/aax_aox_aex.oct", ibus, dbus)

	cpu := newCPU(ibus, dbus)
	cpu.run(true)
}
