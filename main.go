package main

func main() {
	rom := newMemory("ROM", 1024)
	ram := newMemory("RAM", 1024)

	ibus := newBus("IBUS")
	dbus := newBus("DBUS")
	ibus.attach(MemRegion{0, 1023}, &rom)
	dbus.attach(MemRegion{0o2000, 0o2000 + 1023}, &ram)
	loadOct("tests/stack.oct", ibus, dbus)

	cpu := newCPU(ibus, dbus)
	cpu.run()
}
