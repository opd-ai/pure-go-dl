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

func symName(obj *Object, idx uint32) string {
	if idx == 0 || obj.SymtabAddr == 0 || obj.StrtabAddr == 0 {
		return ""
	}
	sym := (*symbol.Elf64Sym)(unsafe.Add(unsafe.Pointer(obj.SymtabAddr), uintptr(idx)*24))
	return symbol.ReadCStringMem(obj.StrtabAddr, uintptr(sym.Name))
}

// symBind returns the symbol binding type (STB_LOCAL, STB_GLOBAL, STB_WEAK, etc.).
func symBind(obj *Object, idx uint32) uint8 {
	if idx == 0 || obj.SymtabAddr == 0 {
		return 0
	}
	sym := (*symbol.Elf64Sym)(unsafe.Add(unsafe.Pointer(obj.SymtabAddr), uintptr(idx)*24))
	return sym.Info >> 4 // upper 4 bits = binding
}

// symAddress returns the symbol's value (address or TLS offset).
func symAddress(obj *Object, idx uint32) uintptr {
	if idx == 0 || obj.SymtabAddr == 0 {
		return 0
	}
	sym := (*symbol.Elf64Sym)(unsafe.Add(unsafe.Pointer(obj.SymtabAddr), uintptr(idx)*24))
	return uintptr(sym.Value)
}
