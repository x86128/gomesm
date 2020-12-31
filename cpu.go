package main

import (
	"fmt"
	"log"
	"math"
)

// CPU state
type CPU struct {
	PC      uint16     // instruction pointer
	pcNext  uint16     // next PC after current instruction execution
	Acc     besmWord   // accumulator
	Rmr     besmWord   // least significand bits register
	M       [16]uint16 // M registers (index registers, address modifiers)
	ibus    *Bus       // instruction bus
	dbus    *Bus       // data bus
	right   bool       // right or left instruction flag
	irCache besmWord   // instruction cache register
	ir      besmWord   // current being executed instruction
	stack   bool       // current instruction in stack mode
	Running bool
	trace   bool // trace instructions

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

func (cpu *CPU) isRLog() bool {
	return cpu.rrReg&0b11100 == 0b100
}

func (cpu *CPU) setRMul() {
	cpu.rrReg = cpu.rrReg&0b11100011 | 0b0001000
}

func (cpu *CPU) setRAdd() {
	cpu.rrReg = cpu.rrReg&0b11100011 | 0b0010000
}

func (cpu *CPU) uAddr() uint16 {
	return (cpu.M[cpu.irIND] + cpu.vAddr) & MASK15
}

func (cpu *CPU) atx() {
	cpu.dbus.write(cpu.uAddr(), cpu.Acc)
	if cpu.stack {
		cpu.M[15] = (cpu.M[15] + 1) & MASK15
	}
}

func (cpu *CPU) stx() {
	cpu.dbus.write(cpu.uAddr(), cpu.Acc)
	cpu.M[15] = (cpu.M[15] - 1) & MASK15
	cpu.Acc = cpu.dbus.read(cpu.M[15])
	cpu.setRLog()
}

func (cpu *CPU) xts() {
	cpu.dbus.write(cpu.M[15], cpu.Acc)
	cpu.M[15] = (cpu.M[15] + 1) & MASK15
	cpu.Acc = cpu.dbus.read(cpu.uAddr())
	cpu.setRLog()
}

func negate(val besmWord) besmWord {
	// unpack number
	exponent := val >> 41 & MASK7
	mantissa := val & MASK41
	// sign extend mantissa
	if mantissa&BIT41 != 0 {
		mantissa |= BIT42
	}
	// two's complement sign-extended mantissa
	mantissa = ((mantissa ^ MASK42) + 1) & MASK42
	// if bits 41 and 42 differs, then shift mantissa 1 bit right
	if ((mantissa>>1)^mantissa)&BIT41 != 0 {
		mantissa = mantissa >> 1
		exponent = (exponent + 1) & MASK7
	}
	return (exponent << 41) + (mantissa & MASK41)
}

func printBesmNumber(val besmWord) string {
	exponent := int16(val>>41&MASK7) - 64 - 40
	mantissa := float64(val & MASK40)
	if val&BIT41 != 0 {
		mantissa = -float64((val ^ MASK41 + 1) & MASK41)
	}

	return fmt.Sprintf("%g (%016o)", mantissa*math.Pow(2.0, float64(exponent)), val)
}

func (cpu *CPU) addsubrsub(a besmWord, b besmWord) {
	doRound := cpu.rrReg&2 == 0
	doNorm := cpu.rrReg&1 == 0
	// 1. unpack
	aExp := a >> 41 & MASK7
	bExp := b >> 41 & MASK7

	aMant := a & MASK41
	if a&BIT41 != 0 {
		aMant |= BIT42
	}

	bMant := b & MASK41
	if b&BIT41 != 0 {
		bMant |= BIT42
	}
	// 2. prepare to mantissa align
	if aExp <= bExp {
		aExp, bExp = bExp, aExp
		aMant, bMant = bMant, aMant
	}
	// reset low 40 bits
	cpu.Rmr = cpu.Rmr & 0o7760000000000000
	// 3. align
	sticky := false
	for aExp != bExp {
		cpu.Rmr = cpu.Rmr >> 1
		if bMant&1 != 0 {
			cpu.Rmr = cpu.Rmr | BIT40
			// when at least one "1"-bit jumps to RMR, after addition
			// set lowest bit of ACC to "one"
			sticky = true
		}
		if bMant&BIT42 != 0 {
			bMant = (bMant >> 1) | BIT42
		} else {
			bMant = bMant >> 1
		}
		bExp++
	}
	// 4.add
	aMant = (aMant + bMant) & MASK42
	rounded := false
	// 5. normalization rounds
	done := false
	for {
		if aExp&0x100 != 0 {
			// exponent underflow
			aExp = 0
			aMant = 0
			cpu.Rmr = 0
			done = true
			break
		} else if (aMant&BIT42 != 0) != (aMant&BIT41 != 0) {
			// shift mantissa to right
			if aMant&1 != 0 {
				sticky = true
				cpu.Rmr = (cpu.Rmr >> 1) | BIT40
			} else {
				cpu.Rmr = cpu.Rmr >> 1
			}

			if aMant&BIT42 != 0 {
				aMant = (aMant >> 1) | BIT42
			} else {
				aMant = aMant >> 1
			}
			aExp++
			if !doRound {
				done = true
			}
			break
		} else if doNorm && ((aMant&BIT41 != 0) == (aMant&BIT40 != 0)) {
			aMant = (aMant << 1) & MASK42
			if cpu.Rmr&BIT40 != 0 {
				rounded = true
				aMant = aMant | 1
			}
			// shift low 40 bits and preserve high 48:41 bits
			cpu.Rmr = (cpu.Rmr & 0xFF0000000000) | ((cpu.Rmr << 1) & MASK40)
			aExp--
		} else {
			if !doRound {
				done = true
			}
			break
		}
	}
	// 6. rounding
	if !done && doRound {
		if (cpu.Rmr&MASK40 != 0 || sticky) && !rounded {
			aMant |= 1
		}
	}
	// 7. pack
	cpu.Acc = ((aExp & MASK7) << 41) | (aMant & MASK41)

}

func (cpu *CPU) add() {
	if cpu.stack {
		cpu.M[15] = (cpu.M[15] - 1) & MASK15
	}

	a := cpu.Acc
	b := cpu.dbus.read(cpu.uAddr())
	cpu.addsubrsub(a, b)

	//log.Println("ADD", printBesmNumber(a), printBesmNumber(b), "=", printBesmNumber(cpu.Acc))

	cpu.setRAdd()
}

func (cpu *CPU) sub() {
	if cpu.stack {
		cpu.M[15] = (cpu.M[15] - 1) & MASK15
	}

	a := cpu.Acc
	b := cpu.dbus.read(cpu.uAddr())

	cpu.addsubrsub(a, negate(b))

	//log.Println("SUB", printBesmNumber(a), printBesmNumber(b), "=", printBesmNumber(cpu.Acc))

	cpu.setRAdd()
}

func (cpu *CPU) rsub() {
	if cpu.stack {
		cpu.M[15] = (cpu.M[15] - 1) & MASK15
	}

	a := cpu.Acc
	b := cpu.dbus.read(cpu.uAddr())

	cpu.addsubrsub(negate(a), b)
	//log.Println("RSUB", printBesmNumber(a), printBesmNumber(b), "=", printBesmNumber(cpu.Acc))

	cpu.setRAdd()
}

func (cpu *CPU) yta() {
	if cpu.isRLog() {
		cpu.Acc = cpu.Rmr
	} else {
		aMant := cpu.Rmr & MASK40
		aExp := ((cpu.Acc >> 41 & MASK7) + besmWord(cpu.uAddr()&MASK7) - 64) & MASK7
		doNorm := cpu.rrReg&1 == 0

		if doNorm {
			for {
				if aExp&0x100 != 0 {
					// exponent underflow
					aExp = 0
					aMant = 0
					cpu.Rmr = 0
					break
				} else if (aMant&BIT42 != 0) != (aMant&BIT41 != 0) {
					// shift mantissa to right
					if aMant&1 != 0 {
						cpu.Rmr = (cpu.Rmr >> 1) | BIT40
					} else {
						cpu.Rmr = cpu.Rmr >> 1
					}

					if aMant&BIT42 != 0 {
						aMant = (aMant >> 1) | BIT42
					} else {
						aMant = aMant >> 1
					}
					aExp++
					break
				} else if doNorm && ((aMant&BIT41 != 0) == (aMant&BIT40 != 0)) {
					aMant = (aMant << 1) & MASK41
					if cpu.Rmr&BIT40 != 0 {
						aMant = aMant | 1
					}
					cpu.Rmr = (cpu.Rmr & 0xFF0000000000) | ((cpu.Rmr << 1) & MASK40)
					aExp--
				} else {
					break
				}
			}
		}
		cpu.Acc = ((aExp & MASK7) << 41) | (aMant & MASK41)
	}
}

func (cpu *CPU) aax() {
	if cpu.stack {
		cpu.M[15] = (cpu.M[15] - 1) & MASK15
	}
	cpu.Acc = cpu.Acc & cpu.dbus.read(cpu.uAddr())
	cpu.Rmr = 0
	cpu.setRLog()
}

func (cpu *CPU) aex() {
	if cpu.stack {
		cpu.M[15] = (cpu.M[15] - 1) & MASK15
	}
	cpu.Rmr = cpu.Acc
	cpu.Acc = cpu.Acc ^ cpu.dbus.read(cpu.uAddr())
	cpu.setRLog()
}

func (cpu *CPU) aox() {
	if cpu.stack {
		cpu.M[15] = (cpu.M[15] - 1) & MASK15
	}
	cpu.Acc = cpu.Acc | cpu.dbus.read(cpu.uAddr())
	cpu.Rmr = 0
	cpu.setRLog()
}

func (cpu *CPU) asn() {
	// shift a <<= Uaddr
	// ALU operand A is ACC
	// ALU operand B is low 7 bits of Uaddr shifted to exponent {uAddr[47:41],41'b0}
	cpu.Rmr = besmWord(0)
	bExp := cpu.uAddr() & MASK7
	if bExp >= 64 {
		// shift right
		for bExp != 64 {
			cpu.Rmr = cpu.Rmr >> 1
			if cpu.Acc&1 != 0 {
				cpu.Rmr |= BIT48
			}
			cpu.Acc = cpu.Acc >> 1
			bExp--
		}
	} else {
		// shift left
		for bExp != 64 {
			cpu.Rmr = cpu.Rmr << 1
			if cpu.Acc&BIT48 != 0 {
				cpu.Rmr |= 1
			}
			cpu.Acc = cpu.Acc << 1
			bExp++
		}
	}
	cpu.setRLog()
}

func (cpu *CPU) asx() {
	if cpu.stack {
		cpu.M[15] = (cpu.M[15] - 1) & MASK15
	}
	cpu.Rmr = besmWord(0)
	bExp := (cpu.dbus.read(cpu.uAddr()) >> 41) & MASK7
	if bExp >= 64 {
		// shift right
		for bExp != 64 {
			cpu.Rmr = cpu.Rmr >> 1
			if cpu.Acc&1 != 0 {
				cpu.Rmr |= BIT48
			}
			cpu.Acc = cpu.Acc >> 1
			bExp--
		}
	} else {
		// shift left
		for bExp != 64 {
			cpu.Rmr = cpu.Rmr << 1
			if cpu.Acc&BIT48 != 0 {
				cpu.Rmr |= 1
			}
			cpu.Acc = cpu.Acc << 1
			bExp++
		}
	}
	cpu.setRLog()
}

func (cpu *CPU) arx() {
	if cpu.stack {
		cpu.M[15] = (cpu.M[15] - 1) & MASK15
	}
	cpu.Acc = cpu.dbus.read(cpu.uAddr()) + cpu.Acc
	if cpu.Acc&BIT49 != 0 {
		cpu.Acc = (cpu.Acc + 1) & MASK48
	}
	cpu.setRMul()
}

func (cpu *CPU) apx() {
	if cpu.stack {
		cpu.M[15] = (cpu.M[15] - 1) & MASK15
	}
	t := besmWord(0)
	cpu.Rmr = cpu.dbus.read(cpu.uAddr())
	bit := 47
	for cpu.Rmr != 0 {
		if cpu.Rmr&BIT48 != 0 {
			if cpu.Acc&BIT48 != 0 {
				t = t | (1 << bit)
			}
			bit--
		}
		cpu.Rmr = cpu.Rmr << 1
		cpu.Acc = cpu.Acc << 1
	}
	cpu.Acc = t
	cpu.setRLog()
}

func (cpu *CPU) aux() {
	if cpu.stack {
		cpu.M[15] = (cpu.M[15] - 1) & MASK15
	}
	t := besmWord(0)
	cpu.Rmr = cpu.dbus.read(cpu.uAddr())
	bit := 47
	for cpu.Rmr != 0 {
		if cpu.Rmr&BIT48 != 0 {
			if cpu.Acc&BIT48 != 0 {
				t = t | (1 << bit)
			}
			cpu.Acc = cpu.Acc << 1

		}
		bit--
		cpu.Rmr = cpu.Rmr << 1
	}
	cpu.Acc = t
	cpu.setRLog()
}

func (cpu *CPU) acx() {
	if cpu.stack {
		cpu.M[15] = (cpu.M[15] - 1) & MASK15
	}
	cpu.Rmr = 0
	t := besmWord(0)
	for i := 0; i < 48; i++ {
		if cpu.Acc&1 != 0 {
			t++
		}
		cpu.Acc = cpu.Acc >> 1
	}
	cpu.Acc = t + cpu.dbus.read(cpu.uAddr())
	if cpu.Acc&BIT49 != 0 {
		cpu.Acc = (cpu.Acc + 1) & MASK48
	}
	cpu.setRLog()
}

func (cpu *CPU) anx() {
	if cpu.stack {
		cpu.M[15] = (cpu.M[15] - 1) & MASK15
	}
	if cpu.Acc == 0 {
		cpu.Rmr = 0
		cpu.Acc = cpu.dbus.read(cpu.uAddr())
	} else {
		bit := besmWord(1)
		for t := cpu.Acc; t != 0 && t&BIT48 == 0; t = t << 1 {
			bit++
		}
		cpu.Rmr = cpu.Acc << bit
		cpu.Acc = bit + cpu.dbus.read(cpu.uAddr())
		if cpu.Acc&BIT49 != 0 {
			cpu.Acc = (cpu.Acc + 1) & MASK48
		}
	}
	cpu.setRLog()
}

func (cpu *CPU) ita() {
	cpu.Acc = besmWord(cpu.M[cpu.uAddr()&0xF])
	cpu.setRLog()
}

func (cpu *CPU) ati() {
	t := cpu.uAddr() & 0xF
	if t != 0 {
		cpu.M[t] = uint16(cpu.Acc & MASK15)
	}
}

func (cpu *CPU) jaddm() {
	t := cpu.vAddr & 0xF
	if t != 0 {
		cpu.M[t] = (cpu.M[t] + cpu.M[cpu.irIND]) & MASK15
	}
}

func (cpu *CPU) utm() {
	t := cpu.irIND
	if t != 0 {
		cpu.M[t] = cpu.uAddr() & MASK15
	}
}

func (cpu *CPU) vjm() {
	if cpu.irIND != 0 {
		cpu.M[cpu.irIND] = cpu.PC + 1 // gotcha :)
	}
	cpu.pcNext = cpu.vAddr
	cpu.right = false
}

func (cpu *CPU) vim() {
	if cpu.M[cpu.irIND] != 0 {
		cpu.pcNext = cpu.vAddr
		cpu.right = false
	}
}

func (cpu *CPU) uj() {
	cpu.pcNext = cpu.uAddr()
	cpu.right = false
}

func (cpu *CPU) xtr() {
	if cpu.stack {
		cpu.M[15] = (cpu.M[15] - 1) & MASK15
	}
	// TODO: in real MESM6 r[5:0] = dbus[46:41]
	// TODO: bin r[6] is "in interrupt" flag - ignoring
	cpu.rrReg = cpu.rrReg&0b1000000 | uint16(cpu.dbus.read(cpu.uAddr())&0o77)
}

func (cpu *CPU) rte() {
	// TODO: in real MESM6 Acc[47:42] = r[5:0] (exponent = r)
	cpu.Acc = besmWord(cpu.rrReg) & 0o77
}

func (cpu *CPU) ntr() {
	// bit 6 of RR register is "in interrupt" flag, so preserve it
	// bits 5 to 0 is set from low 6 bits of uAddr
	cpu.rrReg = (cpu.rrReg & 0b1000000) | (cpu.uAddr() & 0b111111)
}

func (cpu *CPU) xta() {
	if cpu.stack {
		cpu.M[15] = (cpu.M[15] - 1) & MASK15
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
		cpu.right = false
	}
	cpu.Rmr = cpu.Acc
}

func (cpu *CPU) uza() {
	if cpu.accIsZero() {
		cpu.pcNext = cpu.uAddr()
		cpu.right = false
	}
	cpu.Rmr = cpu.Acc
}

func (cpu *CPU) stop() {
	// check for magic stop codes
	// stop 12345(6) - success
	if cpu.irIND == 6 && cpu.irAddr == 0o12345 {
		log.Println("SUCCESS STOP")
	} else if cpu.irIND == 2 && cpu.irAddr == 0o76543 {
		log.Println("FAILED STOP")
	}
	cpu.Running = false
}

func (cpu *CPU) vtm() {
	if cpu.irIND != 0 {
		cpu.M[cpu.irIND] = cpu.vAddr
	}
}

func (cpu *CPU) utc() {
	cpu.cActive = true
	cpu.cReg = cpu.uAddr()
}

func (cpu *CPU) wtc() {
	if cpu.stack {
		cpu.M[15] = (cpu.M[15] - 1) & MASK15
	}
	cpu.cActive = true
	cpu.cReg = uint16(cpu.dbus.read(cpu.uAddr()))
}

func (cpu *CPU) step() {
	// FETCH instruction from cache or
	cpu.ir = cpu.irCache & MASK24
	cpu.pcNext = cpu.PC
	// if last step was executed right instruction
	if !cpu.right {
		// fetch new instruction from insruction bus
		cpu.irCache = cpu.ibus.read(cpu.PC)
		cpu.ir = cpu.irCache >> 24
	} else {
		cpu.pcNext = (cpu.PC + 1) & MASK15
	}
	// print instruction
	if cpu.trace {
		fmt.Println("==== START =====")
		cpu.state()
		fmt.Printf("\nAfter execution of: %s\n", decodeOp(cpu.ir))
	}
	cpu.right = !cpu.right
	// DECODE step 1. unpack instruction
	cpu.irIND = uint16((cpu.ir >> 20) & 0xF)
	if cpu.ir&BIT20 == 0 {
		cpu.irOp = uint16((cpu.ir & 0o770000) >> 12)
		cpu.irAddr = uint16(cpu.ir & 0o7777)
		if cpu.ir&BIT19 != 0 {
			cpu.irAddr |= 0o70000
		}
	} else {
		cpu.irOp = uint16((cpu.ir&0o1700000)>>12 + 0o200)
		cpu.irAddr = uint16(cpu.ir & MASK15)
	}
	// DECODE step 2. modify execution address if needed
	if cpu.cActive {
		cpu.vAddr = (cpu.irAddr + cpu.cReg) & MASK15
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
	case OpSUB:
		cpu.sub()
	case OpADD:
		cpu.add()
	case OpRSUB:
		cpu.rsub()
	case OpYTA:
		cpu.yta()
	case OpAAX:
		cpu.aax()
	case OpAEX:
		cpu.aex()
	case OpAOX:
		cpu.aox()
	case OpASN:
		cpu.asn()
	case OpASX:
		cpu.asx()
	case OpARX:
		cpu.arx()
	case OpAPX:
		cpu.apx()
	case OpAUX:
		cpu.aux()
	case OpACX:
		cpu.acx()
	case OpANX:
		cpu.anx()
	case OpNTR:
		cpu.ntr()
	case OpUIA:
		cpu.uia()
	case OpUZA:
		cpu.uza()
	case OpSTOP:
		cpu.stop()
	case OpVTM:
		cpu.vtm()
	case OpITA:
		cpu.ita()
	case OpATI:
		cpu.ati()
	case OpJADDM:
		cpu.jaddm()
	case OpUTM:
		cpu.utm()
	case OpVJM:
		cpu.vjm()
	case OpVIM:
		cpu.vim()
	case OpUJ:
		cpu.uj()
	case OpUTC:
		cpu.utc()
	case OpWTC:
		cpu.wtc()
	default:
		log.Printf("Unimplemented opcode: %03o - %s", cpu.irOp, decodeOp(cpu.ir))
		cpu.Running = false
	}
	// advance instrunction pointer
	cpu.PC = cpu.pcNext

	if cpu.trace {
		cpu.state()
		fmt.Println("=== END ===")
	}
}

func (cpu *CPU) state() {
	fmt.Printf("PC:\t%05o right: %t IR: %08o %s\n", cpu.PC, cpu.right, cpu.ir, decodeOp(cpu.ir))
	fmt.Printf("M:\t%05o\n", cpu.M)
	fmt.Printf("ACC:\t%016o RMR:%016o\n", cpu.Acc, cpu.Rmr)
	fmt.Printf("RR: %07b\n", cpu.rrReg)
	fmt.Printf("cActive: %t cReg: %05o\n", cpu.cActive, cpu.cReg)
}

func (cpu *CPU) run() {
	cpu.Running = true
	for cpu.Running {
		cpu.step()
	}
}

func newCPU(ibus *Bus, dbus *Bus) *CPU {
	cpu := CPU{}
	cpu.ibus = ibus
	cpu.dbus = dbus
	cpu.reset()
	return &cpu
}
