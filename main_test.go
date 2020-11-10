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
