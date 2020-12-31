package main

import (
	"fmt"
)

// various masks
const (
	BIT19 = 0o1000000
	BIT20 = 0o2000000
	BIT40 = 0o0010000000000000
	BIT41 = 0o0020000000000000
	BIT42 = 0o0040000000000000
	//         ____====____====
	BIT48  = 0o4000000000000000
	BIT49  = 0o10000000000000000
	MASK7  = 0o177
	MASK12 = 0o7777
	MASK15 = 0o77777
	MASK24 = 0o77777777
	MASK40 = 0x00FFFFFFFFFF
	MASK41 = 0x01FFFFFFFFFF
	MASK42 = 0x03FFFFFFFFFF
	MASK48 = 0o7777777777777777
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
	addr = addr & MASK15
	if op <= OpE77 {
		if addr >= 0o7777 && addr <= 0o67777 {
			return 0, fmt.Errorf("Address: %05o out of range for `short` command", addr)
		}
		word |= (besmWord(op) << 12) + (besmWord(addr) & MASK12)
		if addr >= 0o70000 {
			word |= BIT19 // set address extension BIT19 = 1
		}
	} else {
		word |= (besmWord(op) << 12) + (besmWord(addr) & MASK15)
	}
	return word, nil
}

var opShortNames = [...]string{
	"ATX", "STX", "MOD", "XTS", "ADD", "SUB", "RSUB", "AMX",
	"XTA", "AAX", "AEX", "ARX", "AVX", "AOX", "DIV", "MUL",
	"APX", "AUX", "ACX", "ANX", "EADDX", "ESUBX", "ASX", "XTR",
	"RTE", "YTA", "E32", "E33", "EADDN", "ESUB", "ASN", "NTR",
	"ATI", "STI", "ITA", "ITS", "MTJ", "JADDM", "E46", "E47",
	"E50", "E51", "E52", "E53", "E54", "E55", "E56", "E57",
	"E60", "E61", "E62", "E63", "E64", "E65", "E66", "E67",
	"E70", "E71", "E72", "E73", "E74", "E75", "E76", "E77",
}

var opLongNames = [...]string{
	"E20", "E21", "UTC", "WTC", "VTM", "UTM", "UZA", "UIA",
	"UJ", "VJM", "IJ", "STOP", "VZM", "VIM", "E36", "VLM",
}

func decodeOp(word besmWord) (res string) {
	word &= 0o77777777
	if word&BIT20 == 0 {
		addr := word & MASK12
		op := (word & 0o770000) >> 12
		ind := (word & 0o74000000) >> 20
		if word&BIT19 != 0 {
			addr |= 0o70000
		}
		res = fmt.Sprintf("%s %o(%o)", opShortNames[op], addr, ind)
	} else {
		// long address command
		addr := word & MASK15
		op := (word & 0o3700000) >> 15
		ind := (word & 0o74000000) >> 20
		res = fmt.Sprintf("%s %o(%o)", opLongNames[op-0o20], addr, ind)
	}
	return res
}
