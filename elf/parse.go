// Package elf extends the standard library's debug/elf with additional ELF parsing
// functionality needed for dynamic linking, including GNU hash tables, symbol versioning,
// and dynamic section parsing.
package elf

import (
	"debug/elf"
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

// PageAlign rounds v up to the next 4096-byte page boundary.
func PageAlign(v uint64) uint64 {
	return (v + 4095) &^ 4095
}

// ParsedObject holds everything extracted from a shared-object ELF file.
type ParsedObject struct {
	Path string
	File *elf.File

	// Computed address span
	BaseVAddr uint64 // minimum vaddr of PT_LOAD segments
	MemSize   uint64 // total span, page-aligned

	// Program headers by type
	LoadSegments []elf.ProgHeader
	DynamicSeg   *elf.ProgHeader // PT_DYNAMIC
	GNURelroSeg  *elf.ProgHeader // PT_GNU_RELRO (may be nil)

	// Dynamic section entries (tag -> value)
	DynEntries map[elf.DynTag]uint64

	// DT_NEEDED library names
	Needed []string

	// DT_RUNPATH and DT_RPATH search paths (colon-separated, may be empty)
	Runpath string // DT_RUNPATH (modern, preferred)
	Rpath   string // DT_RPATH (legacy)

	// Raw dynamic data
	DynData  []byte
	DynVAddr uint64
}

// Parse opens the ELF shared object at path and extracts metadata needed for
// loading. It validates that the file is a 64-bit x86-64 shared library.
func Parse(path string) (*ParsedObject, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("elf parse: open %q: %w", path, err)
	}
	defer f.Close()

	ef, err := elf.NewFile(f)
	if err != nil {
		return nil, fmt.Errorf("elf parse: %q: %w", path, err)
	}

	if ef.Class != elf.ELFCLASS64 {
		return nil, fmt.Errorf("elf parse: %q: not a 64-bit ELF", path)
	}
	if ef.Machine != elf.EM_X86_64 {
		return nil, fmt.Errorf("elf parse: %q: not x86-64 (got %v)", path, ef.Machine)
	}
	if ef.Type != elf.ET_DYN {
		return nil, fmt.Errorf("elf parse: %q: not a shared object (type %v)", path, ef.Type)
	}

	obj := &ParsedObject{
		Path:       path,
		File:       ef,
		DynEntries: make(map[elf.DynTag]uint64),
	}

	// Collect program headers.
	var minVAddr, maxVAddr uint64
	first := true
	for i := range ef.Progs {
		ph := ef.Progs[i]
		switch ph.Type {
		case elf.PT_LOAD:
			obj.LoadSegments = append(obj.LoadSegments, ph.ProgHeader)
			end := ph.Vaddr + ph.Memsz
			if first {
				minVAddr = ph.Vaddr
				maxVAddr = end
				first = false
			} else {
				if ph.Vaddr < minVAddr {
					minVAddr = ph.Vaddr
				}
				if end > maxVAddr {
					maxVAddr = end
				}
			}
		case elf.PT_DYNAMIC:
			hdr := ph.ProgHeader
			obj.DynamicSeg = &hdr
		case elf.PT_GNU_RELRO:
			hdr := ph.ProgHeader
			obj.GNURelroSeg = &hdr
		}
	}

	if len(obj.LoadSegments) == 0 {
		return nil, fmt.Errorf("elf parse: %q: no PT_LOAD segments", path)
	}
	if obj.DynamicSeg == nil {
		return nil, fmt.Errorf("elf parse: %q: no PT_DYNAMIC segment", path)
	}

	obj.BaseVAddr = minVAddr
	obj.MemSize = PageAlign(maxVAddr - minVAddr)

	// Read the dynamic segment data from the file.
	dynProg := obj.DynamicSeg
	obj.DynVAddr = dynProg.Vaddr

	dynData := make([]byte, dynProg.Filesz)
	if _, err := f.ReadAt(dynData, int64(dynProg.Off)); err != nil && err != io.EOF {
		return nil, fmt.Errorf("elf parse: %q: read dynamic segment: %w", path, err)
	}
	obj.DynData = dynData

	// Parse dynamic section entries (Elf64_Dyn: 8-byte tag + 8-byte value).
	const dynEntSize = 16
	for off := 0; off+dynEntSize <= len(dynData); off += dynEntSize {
		tag := elf.DynTag(binary.LittleEndian.Uint64(dynData[off:]))
		val := binary.LittleEndian.Uint64(dynData[off+8:])
		if tag == elf.DT_NULL {
			break
		}
		obj.DynEntries[tag] = val
	}

	// Resolve DT_NEEDED names using the string table embedded in the file.
	// DT_STRTAB is a virtual address; we resolve it by scanning PT_LOAD segments.
	if strtabVA, ok := obj.DynEntries[elf.DT_STRTAB]; ok {
		strtabData, err := readBytesAtVAddr(f, ef, strtabVA)
		if err != nil {
			return nil, fmt.Errorf("elf parse: %q: read DT_STRTAB: %w", path, err)
		}
		// Collect all DT_NEEDED entries (may appear multiple times — iterate raw).
		for off := 0; off+dynEntSize <= len(dynData); off += dynEntSize {
			tag := elf.DynTag(binary.LittleEndian.Uint64(dynData[off:]))
			if tag == elf.DT_NULL {
				break
			}
			if tag == elf.DT_NEEDED {
				nameOff := int(binary.LittleEndian.Uint64(dynData[off+8:]))
				name := readCString(strtabData, nameOff)
				obj.Needed = append(obj.Needed, name)
			}
		}

		// Parse DT_RUNPATH and DT_RPATH from the string table.
		if runpathOff, ok := obj.DynEntries[elf.DT_RUNPATH]; ok {
			obj.Runpath = readCString(strtabData, int(runpathOff))
		}
		if rpathOff, ok := obj.DynEntries[elf.DT_RPATH]; ok {
			obj.Rpath = readCString(strtabData, int(rpathOff))
		}
	}

	return obj, nil
}

// readBytesAtVAddr locates the PT_LOAD segment that covers vaddr and reads
// the remainder of that segment's file data starting at vaddr.
func readBytesAtVAddr(f *os.File, ef *elf.File, vaddr uint64) ([]byte, error) {
	for _, ph := range ef.Progs {
		if ph.Type != elf.PT_LOAD {
			continue
		}
		if vaddr >= ph.Vaddr && vaddr < ph.Vaddr+ph.Filesz {
			off := ph.Off + (vaddr - ph.Vaddr)
			size := ph.Filesz - (vaddr - ph.Vaddr)
			buf := make([]byte, size)
			n, err := f.ReadAt(buf, int64(off))
			if err != nil && err != io.EOF {
				return nil, err
			}
			return buf[:n], nil
		}
	}
	return nil, fmt.Errorf("vaddr 0x%x not found in any PT_LOAD segment", vaddr)
}

// readCString reads a null-terminated string from data starting at off.
func readCString(data []byte, off int) string {
	if off < 0 || off >= len(data) {
		return ""
	}
	end := off
	for end < len(data) && data[end] != 0 {
		end++
	}
	return string(data[off:end])
}
