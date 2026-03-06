// Package symbol provides symbol table parsing, hashing, and lookup for ELF shared objects.
// It supports both GNU hash and SysV hash tables, as well as GNU symbol versioning.
package symbol

import (
	"debug/elf"
	"fmt"
	"unsafe"
)

// GnuHash computes the GNU ELF hash of a symbol name.
func GnuHash(name string) uint32 {
	h := uint32(5381)
	for i := 0; i < len(name); i++ {
		h = h*33 + uint32(name[i])
	}
	return h
}

// GnuLookup looks up a symbol by name using the DT_GNU_HASH table.
//
// gnuHashAddr – in-memory address of the GNU hash table
// symtabAddr  – in-memory address of DT_SYMTAB
// strtabAddr  – in-memory address of DT_STRTAB
//
// Note: This function works with mmap'd ELF file memory. The uintptr→unsafe.Pointer
// conversions flagged by go vet are safe because:
// 1. The addresses come from mmap and remain valid for the lifetime of the mapping
// 2. The memory is pinned and won't be moved by the GC
// 3. We convert to unsafe.Pointer immediately before dereferencing
func GnuLookup(name string, gnuHashAddr, symtabAddr, strtabAddr uintptr) (*Symbol, error) {
	if gnuHashAddr == 0 {
		return nil, fmt.Errorf("gnu_hash: hash table address is 0")
	}

	// GNU hash table header layout:
	//   uint32 nbuckets
	//   uint32 symoffset
	//   uint32 bloom_size  (number of 64-bit bloom words)
	//   uint32 bloom_shift
	hashPtr := unsafe.Pointer(gnuHashAddr)
	nbuckets := *(*uint32)(hashPtr)
	symoffset := *(*uint32)(unsafe.Add(hashPtr, 4))
	bloomSize := *(*uint32)(unsafe.Add(hashPtr, 8))
	bloomShift := *(*uint32)(unsafe.Add(hashPtr, 12))

	bloomBase := gnuHashAddr + 16
	bloomBasePtr := unsafe.Pointer(bloomBase)
	bucketsBase := bloomBase + uintptr(bloomSize)*8
	bucketsBasePtr := unsafe.Pointer(bucketsBase)
	chainsBase := bucketsBase + uintptr(nbuckets)*4
	chainsBasePtr := unsafe.Pointer(chainsBase)

	h := GnuHash(name)

	// Bloom filter check.
	bloomWord := *(*uint64)(unsafe.Add(bloomBasePtr, uintptr((h/64)%uint32(bloomSize))*8))
	bit1 := (h >> 0) & 63
	bit2 := (h >> bloomShift) & 63
	if (bloomWord>>bit1)&1 == 0 || (bloomWord>>bit2)&1 == 0 {
		return nil, fmt.Errorf("gnu_hash: symbol %q not found (bloom)", name)
	}

	// Bucket lookup.
	bucket := *(*uint32)(unsafe.Add(bucketsBasePtr, uintptr(h%uint32(nbuckets))*4))
	if bucket == 0 {
		return nil, fmt.Errorf("gnu_hash: symbol %q not found (empty bucket)", name)
	}

	// Walk the chain.
	for symIdx := bucket; ; symIdx++ {
		chainVal := *(*uint32)(unsafe.Add(chainsBasePtr, uintptr(symIdx-symoffset)*4))
		if (chainVal &^ 1) == (h &^ 1) {
			sym := symAtIndex(symtabAddr, uintptr(symIdx))
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
		}
		if chainVal&1 != 0 {
			break // end-of-chain flag
		}
	}

	return nil, fmt.Errorf("gnu_hash: symbol %q not found", name)
}
