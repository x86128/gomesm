package main

import "testing"

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

func TestFetch(t *testing.T) {
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
