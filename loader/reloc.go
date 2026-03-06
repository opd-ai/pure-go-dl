package loader

import (
	"unsafe"

	"github.com/opd-ai/pure-go-dl/symbol"
)

// relaEntry mirrors Elf64_Rela: offset(8) + info(8) + addend(8).
type relaEntry struct {
	Offset uint64
	Info   uint64
	Addend int64
}

// getSymbolEntry returns a pointer to the symbol at idx, or nil if invalid.
func getSymbolEntry(obj *Object, idx uint32) *symbol.Elf64Sym {
	if idx == 0 || obj.SymtabAddr == 0 {
		return nil
	}
	symCount := obj.SymtabSize / 24
	if uint64(idx) >= symCount {
		return nil
	}
	return (*symbol.Elf64Sym)(unsafe.Add(unsafe.Pointer(obj.SymtabAddr), uintptr(idx)*24))
}

func symName(obj *Object, idx uint32) string {
	if obj.StrtabAddr == 0 {
		return ""
	}
	sym := getSymbolEntry(obj, idx)
	if sym == nil {
		return ""
	}
	return symbol.ReadCStringMem(obj.StrtabAddr, uintptr(sym.Name), uintptr(obj.StrtabSize))
}

// symBind returns the symbol binding type (STB_LOCAL, STB_GLOBAL, STB_WEAK, etc.).
func symBind(obj *Object, idx uint32) uint8 {
	sym := getSymbolEntry(obj, idx)
	if sym == nil {
		return 0
	}
	return sym.Info >> 4 // upper 4 bits = binding
}

// symAddress returns the symbol's value (address or TLS offset).
func symAddress(obj *Object, idx uint32) uintptr {
	sym := getSymbolEntry(obj, idx)
	if sym == nil {
		return 0
	}
	return uintptr(sym.Value)
}
