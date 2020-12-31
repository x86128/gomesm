package main

import "fmt"

func main() {
	rom := newMemory("ROM", 1024)
	ram := newMemory("RAM", 1024)

	ibus := newBus("IBUS")
	dbus := newBus("DBUS")
	ibus.attach(MemRegion{0, 1023}, &rom)
	dbus.attach(MemRegion{0o2000, 0o2000 + 1023}, &ram)
	cpu := newCPU(ibus, dbus)

	tests := []string{"tests/a+x_a-x_x-a.oct", "tests/aax_aox_aex.oct", "tests/addr0.oct", "tests/apx_aux.oct", "tests/stack.oct"}
	for _, t := range tests {
		fmt.Println("Begin of:", t)
		cpu.reset()
		loadOct(t, ibus, dbus)
		cpu.run()
	}
}
