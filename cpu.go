package main

import (
	"fmt"
	"log"
)

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
	fmt.Printf("PC:\t%05o right: %t IR: %08o %s\n", cpu.PC, cpu.right, cpu.ir, decodeOp(cpu.ir))
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
