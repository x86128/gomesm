package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
)

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
