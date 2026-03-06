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

// gnuHashHeader holds the parsed GNU hash table header.
type gnuHashHeader struct {
	nbuckets    uint32
	symoffset   uint32
	bloomSize   uint32
	bloomShift  uint32
	bloomBase   uintptr
	bucketsBase uintptr
	chainsBase  uintptr
}

// parseGnuHashHeader extracts the GNU hash table structure.
func parseGnuHashHeader(gnuHashAddr uintptr) gnuHashHeader {
	hashPtr := unsafe.Pointer(gnuHashAddr)
	nbuckets := *(*uint32)(hashPtr)
	symoffset := *(*uint32)(unsafe.Add(hashPtr, 4))
	bloomSize := *(*uint32)(unsafe.Add(hashPtr, 8))
	bloomShift := *(*uint32)(unsafe.Add(hashPtr, 12))

	bloomBase := gnuHashAddr + 16
	bucketsBase := bloomBase + uintptr(bloomSize)*8
	chainsBase := bucketsBase + uintptr(nbuckets)*4

	return gnuHashHeader{
		nbuckets:    nbuckets,
		symoffset:   symoffset,
		bloomSize:   bloomSize,
		bloomShift:  bloomShift,
		bloomBase:   bloomBase,
		bucketsBase: bucketsBase,
		chainsBase:  chainsBase,
	}
}

// checkBloomFilter performs a Bloom filter test for a hash value.
func checkBloomFilter(h uint32, header gnuHashHeader) bool {
	bloomBasePtr := unsafe.Pointer(header.bloomBase)
	bloomWord := *(*uint64)(unsafe.Add(bloomBasePtr, uintptr((h/64)%header.bloomSize)*8))
	bit1 := (h >> 0) & 63
	bit2 := (h >> header.bloomShift) & 63
	return (bloomWord>>bit1)&1 != 0 && (bloomWord>>bit2)&1 != 0
}

// searchHashChain walks the GNU hash chain looking for a matching symbol.
func searchHashChain(name string, h, bucket uint32, header gnuHashHeader, symtabAddr, strtabAddr uintptr, strtabSize uint64) (*Symbol, error) {
	chainsBasePtr := unsafe.Pointer(header.chainsBase)
	for symIdx := bucket; ; symIdx++ {
		chainVal := *(*uint32)(unsafe.Add(chainsBasePtr, uintptr(symIdx-header.symoffset)*4))
		if (chainVal &^ 1) == (h &^ 1) {
			sym := symAtIndex(symtabAddr, uintptr(symIdx))
			symName := ReadCStringMem(strtabAddr, uintptr(sym.Name), uintptr(strtabSize))
			if symName == name {
				bind := elf.SymBind(sym.Info >> 4)
				symType := elf.SymType(sym.Info & 0xf)
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
			break
		}
	}
	return nil, fmt.Errorf("gnu_hash: symbol %q not found", name)
}

// GnuLookup looks up a symbol by name using the DT_GNU_HASH table.
//
// gnuHashAddr – in-memory address of the GNU hash table
// symtabAddr  – in-memory address of DT_SYMTAB
// strtabAddr  – in-memory address of DT_STRTAB
// strtabSize  – size of the string table in bytes (for bounds checking)
//
// Note: This function works with mmap'd ELF file memory. The uintptr→unsafe.Pointer
// conversions flagged by go vet are safe because:
// 1. The addresses come from mmap and remain valid for the lifetime of the mapping
// 2. The memory is pinned and won't be moved by the GC
// 3. We convert to unsafe.Pointer immediately before dereferencing
func GnuLookup(name string, gnuHashAddr, symtabAddr, strtabAddr uintptr, strtabSize uint64) (*Symbol, error) {
	if gnuHashAddr == 0 {
		return nil, fmt.Errorf("gnu_hash: hash table address is 0")
	}

	header := parseGnuHashHeader(gnuHashAddr)
	h := GnuHash(name)

	if !checkBloomFilter(h, header) {
		return nil, fmt.Errorf("gnu_hash: symbol %q not found (bloom)", name)
	}

	bucketsBasePtr := unsafe.Pointer(header.bucketsBase)
	bucket := *(*uint32)(unsafe.Add(bucketsBasePtr, uintptr(h%header.nbuckets)*4))
	if bucket == 0 {
		return nil, fmt.Errorf("gnu_hash: symbol %q not found (empty bucket)", name)
	}

	return searchHashChain(name, h, bucket, header, symtabAddr, strtabAddr, strtabSize)
}
