package main

import (
	"io/ioutil"
	"log"
	"testing"
)

func init() {
	log.SetOutput(ioutil.Discard)
}
func TestRam(t *testing.T) {
	ram := newMemory("RAM", 1024)

	ram.write(0, 10)
	if ram.read(0) != 10 {
		t.Error("Excpected 10")
	}

	if ram.read(1025) != 0xDEADBEEF {
		t.Error("Read from out of bounds failed")
	}

}

func TestBus(t *testing.T) {
	bus := newBus("IBUS")

	if bus.read(0) != 0xDEADBEEF {
		t.Error("Read from out of bounds failed")
	}
}

func TestBusWithRam(t *testing.T) {
	bus := newBus("IBUS")
	ram := newMemory("RAM", 1024)

	bus.attach(MemRegion{0, 1023}, &ram)

	bus.write(0, 10)
	if ram.read(0) != 10 {
		t.Error("Bus write than read from ram failed")
	}

	ram.write(1, 20)
	if bus.read(1) != 20 {
		t.Error("Ram write than read from bus failed")
	}
}

func TestStep(t *testing.T) {
	rom := newMemory("ROM", 1024)
	ibus := newBus("IBUS")
	ibus.attach(MemRegion{0, 2047}, &rom)
	cpu := newCPU(ibus, ibus)
	rom.write(1, 0o6666666677777777)
	cpu.step()
	if cpu.irCache != 0o6666666677777777 {
		t.Error("Fetch to IR cache failed")
	}
	if cpu.ir != 0o66666666 {
		t.Error("IR load error")
	}

	cpu.step()
	if cpu.ir != 0o77777777 {
		t.Error("IR load from cache error")
	}
}

func TestPCIncrement(t *testing.T) {
	rom := newMemory("ROM", 16)
	ibus := newBus("IBUS")
	ibus.attach(MemRegion{0, 15}, &rom)
	cpu := newCPU(ibus, ibus)
	cpu.step()
	if cpu.PC != 1 {
		t.Error("PC incremented when executing left command")
	}
	cpu.step()
	if cpu.PC != 2 {
		t.Error("PC not incremented after executing right command")
	}
}

func TestCPUReset(t *testing.T) {
	ibus := newBus("IBUS")
	cpu := newCPU(ibus, ibus)
	cpu.M[4] = 4
	cpu.reset()
	if cpu.PC != 1 {
		t.Error("PC is not equal to 1")
	}
	if cpu.right != false {
		t.Error("Execution need to start from left instruction")
	}
	if cpu.M[4] != 0 {
		t.Error("CPU M registers no cleared")
	}
}

func TestEmitOp(t *testing.T) {
	w, e := emitOp(15, OpATX, 0o20000)
	if e == nil {
		t.Error("Short address op emited with out of range address")
	}

	// check reg and adress field
	w, e = emitOp(15, OpATX, 0o10)
	if e == nil && w != 0o74000010 {
		t.Errorf("ATX 10(17) emit error. W: %08o", w)
	}

	// check adress >= 0o70000 for short address cmd
	w, e = emitOp(6, OpXTA, 0o71234)
	if e == nil && w != 0o31101234 {
		t.Errorf("XTA 71234(6) emit error. W: %08o %024b", w, w)
	}

	// check long adress command
	w, e = emitOp(8, OpSTOP, 0o54321)
	if e == nil && w != 0o43354321 {
		t.Errorf("STOP 54321(10) emit error. W: %08o %024b", w, w)
	}
}

func TestATXXTA(t *testing.T) {
	mem := newMemory("MEM", 1024)
	ibus := newBus("IBUS")
	ibus.attach(MemRegion{0, 1023}, &mem)
	cpu := newCPU(ibus, ibus)
	cpu.ACC = 0xC0DE
	// normal mode
	instr, _ := emitOp(0, OpATX, 55)
	mem.write(1, instr<<24)
	cpu.step()
	if mem.read(55) != 0xC0DE {
		t.Error("normal mode ATX is not working")
	}
	// stack mode
	cpu.reset()
	cpu.M[15] = 15
	cpu.ACC = 12345
	instr, _ = emitOp(15, OpATX, 0)
	mem.write(1, instr<<24)
	cpu.step()
	if mem.read(15) != 12345 {
		t.Error("Stack mode ATX is not working")
	}
	// test xta
	cpu.reset()
	instr, _ = emitOp(0, OpXTA, 55)
	mem.write(1, instr<<24)
	cpu.step()
	if cpu.ACC != 0xC0DE {
		t.Error("Normal mode XTA not mowrking")
	}
	if cpu.rrReg&4 == 0 {
		t.Error("Logial mode flag is not set")
	}
	// test stack mode xta
	cpu.reset()
	cpu.M[15] = 56
	instr, _ = emitOp(15, OpXTA, 0)
	mem.write(1, instr<<24)
	cpu.step()
	if cpu.ACC != 0xC0DE {
		t.Error("Stack mode XTA not mowrking")
	}
	if cpu.rrReg&4 == 0 {
		t.Error("Logial mode flag is not set")
	}
}
