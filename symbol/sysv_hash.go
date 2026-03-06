package symbol

import (
	"debug/elf"
	"fmt"
	"unsafe"
)

// SysvHash computes the SysV ELF hash of a symbol name.
func SysvHash(name string) uint32 {
	var h uint32
	for i := 0; i < len(name); i++ {
		h = (h << 4) + uint32(name[i])
		if g := h & 0xf0000000; g != 0 {
			h ^= g >> 24
		}
		h &^= 0xf0000000
	}
	return h
}

// SysvLookup looks up a symbol by name using the DT_HASH (SysV) table.
//
// hashAddr   – in-memory address of the hash table
// symtabAddr – in-memory address of DT_SYMTAB
// strtabAddr – in-memory address of DT_STRTAB
func SysvLookup(name string, hashAddr, symtabAddr, strtabAddr uintptr) (*Symbol, error) {
	if hashAddr == 0 {
		return nil, fmt.Errorf("sysv_hash: hash table address is 0")
	}

	hashPtr := unsafe.Pointer(hashAddr)
	nbuckets := *(*uint32)(hashPtr)
	// nchains follows immediately
	// nchains := *(*uint32)(unsafe.Add(hashPtr, 4))

	bucketsBase := hashAddr + 8
	bucketsBasePtr := unsafe.Pointer(bucketsBase)
	// chains start after buckets
	chainsBase := bucketsBase + uintptr(nbuckets)*4
	chainsBasePtr := unsafe.Pointer(chainsBase)

	h := SysvHash(name) % nbuckets
	idx := *(*uint32)(unsafe.Add(bucketsBasePtr, uintptr(h)*4))

	for idx != 0 {
		sym := symAtIndex(symtabAddr, uintptr(idx))
		symName := ReadCStringMem(strtabAddr, uintptr(sym.Name))
		if symName == name {
			bind := elf.SymBind(sym.Info >> 4)
			symType := elf.SymType(sym.Info & 0xf)
			// Value holds the virtual address (st_value); callers must add the
			// library's load base to obtain the runtime address.
			return &Symbol{
				Name:    name,
				Value:   uintptr(sym.Value),
				Size:    sym.Size,
				Bind:    bind,
				Type:    symType,
				Section: elf.SectionIndex(sym.Shndx),
			}, nil
		}
		idx = *(*uint32)(unsafe.Add(chainsBasePtr, uintptr(idx)*4))
	}

	return nil, fmt.Errorf("sysv_hash: symbol %q not found", name)
}

// symAtIndex returns a pointer to the Elf64Sym at the given index.
func symAtIndex(symtabAddr, idx uintptr) *Elf64Sym {
	return (*Elf64Sym)(unsafe.Add(unsafe.Pointer(symtabAddr), idx*symEntSize))
}
