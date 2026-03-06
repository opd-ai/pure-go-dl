package symbol_test

import (
	"debug/elf"
	"testing"
	"unsafe"

	"github.com/opd-ai/pure-go-dl/symbol"
)

func TestSysvHash(t *testing.T) {
	tests := []struct {
		name string
		want uint32
	}{
		{"", 0x00000000},
		{"printf", 0x077905a6},
		{"malloc", 0x07383353},
		{"free", 0x0006d8b5},
		{"cos", 0x00006a63},
	}
	for _, tt := range tests {
		got := symbol.SysvHash(tt.name)
		if got != tt.want {
			t.Errorf("SysvHash(%q) = 0x%x, want 0x%x", tt.name, got, tt.want)
		}
	}
}

func TestGnuHash(t *testing.T) {
	tests := []struct {
		name string
		want uint32
	}{
		{"", 5381},
		{"printf", 0x156b2bb8},
	}
	for _, tt := range tests {
		got := symbol.GnuHash(tt.name)
		if got != tt.want {
			t.Errorf("GnuHash(%q) = 0x%x, want 0x%x", tt.name, got, tt.want)
		}
	}
}

func TestSysvLookup(t *testing.T) {
	// Create a minimal SysV hash table in memory.
	// Structure:
	//   uint32 nbuckets
	//   uint32 nchains
	//   uint32 buckets[nbuckets]
	//   uint32 chains[nchains]

	// String table: "\x00cos\x00sin\x00"
	strtab := []byte("\x00cos\x00sin\x00")
	strtabAddr := uintptr(unsafe.Pointer(&strtab[0]))

	// Symbol table with 3 entries (0=NULL, 1=cos, 2=sin)
	const symEntSize = 24
	symbolBuf := make([]byte, 10*symEntSize)
	symbols := (*[10]symbol.Elf64Sym)(unsafe.Pointer(&symbolBuf[0]))
	
	// Symbol 0: NULL
	symbols[0] = symbol.Elf64Sym{}
	
	// Symbol 1: cos
	symbols[1] = symbol.Elf64Sym{
		Name:  1,
		Info:  byte(uint8(elf.STB_GLOBAL)<<4 | uint8(elf.STT_FUNC)&0xf),
		Shndx: 1,
		Value: 0x1000,
		Size:  16,
	}
	
	// Symbol 2: sin
	symbols[2] = symbol.Elf64Sym{
		Name:  5,
		Info:  byte(uint8(elf.STB_GLOBAL)<<4 | uint8(elf.STT_FUNC)&0xf),
		Shndx: 1,
		Value: 0x2000,
		Size:  16,
	}
	
	symtabAddr := uintptr(unsafe.Pointer(&symbols[0]))

	// Create SysV hash table:
	// nbuckets=2, nchains=3
	// Hash "sin" = 0x79fe % 2 = 0
	// Hash "cos" = 0x6a63 % 2 = 1
	// buckets[0] = 2 (points to "sin")
	// buckets[1] = 1 (points to "cos")
	// chains[0] = 0
	// chains[1] = 0 (end of chain)
	// chains[2] = 0 (end of chain)
	hashTable := []uint32{
		2, // nbuckets
		3, // nchains
		2, // buckets[0] -> "sin"
		1, // buckets[1] -> "cos"
		0, // chains[0]
		0, // chains[1]
		0, // chains[2]
	}
	hashAddr := uintptr(unsafe.Pointer(&hashTable[0]))

	// Test successful lookup of "cos"
	sym, err := symbol.SysvLookup("cos", hashAddr, symtabAddr, strtabAddr)
	if err != nil {
		t.Fatalf("SysvLookup(cos) failed: %v", err)
	}
	if sym.Name != "cos" {
		t.Errorf("Symbol name = %q, want cos", sym.Name)
	}
	if sym.Value != 0x1000 {
		t.Errorf("Symbol value = %#x, want 0x1000", sym.Value)
	}
	if sym.Type != elf.STT_FUNC {
		t.Errorf("Symbol type = %v, want STT_FUNC", sym.Type)
	}

	// Test successful lookup of "sin"
	sym, err = symbol.SysvLookup("sin", hashAddr, symtabAddr, strtabAddr)
	if err != nil {
		t.Fatalf("SysvLookup(sin) failed: %v", err)
	}
	if sym.Name != "sin" {
		t.Errorf("Symbol name = %q, want sin", sym.Name)
	}
	if sym.Value != 0x2000 {
		t.Errorf("Symbol value = %#x, want 0x2000", sym.Value)
	}

	// Test failed lookup
	_, err = symbol.SysvLookup("nonexistent", hashAddr, symtabAddr, strtabAddr)
	if err == nil {
		t.Error("SysvLookup should fail for nonexistent symbol")
	}
}

func TestSysvLookup_ZeroHashAddr(t *testing.T) {
	_, err := symbol.SysvLookup("test", 0, 0x1000, 0x2000)
	if err == nil {
		t.Error("SysvLookup with zero hash address should return error")
	}
}
