package loader

import (
	"debug/elf"
	"fmt"
	"os"
	"unsafe"

	goelf "github.com/opd-ai/pure-go-dl/elf"
	"github.com/opd-ai/pure-go-dl/internal/mmap"
	"github.com/opd-ai/pure-go-dl/internal/tls"
	"github.com/opd-ai/pure-go-dl/symbol"
	"golang.org/x/sys/unix"
)

// SymbolResolver resolves symbol names to absolute addresses.
type SymbolResolver interface {
	Resolve(name string) (uintptr, error)
	// ResolveWithLibrary resolves a symbol and returns both its address and the
	// providing library's Object (for TLS module tracking). Returns nil Object
	// if the symbol is internally resolved or provided by the runtime.
	ResolveWithLibrary(name string) (uintptr, *Object, error)
}

// Segment describes a single mapped PT_LOAD region.
type Segment struct {
	Addr     uintptr
	Size     uintptr
	Prot     int
	FileOff  uint64
	MemSize  uint64
	FileSize uint64
}

// Object holds the fully loaded shared object.
type Object struct {
	Parsed   *goelf.ParsedObject
	Base     uintptr       // actual load base address
	Segments []Segment     // one entry per PT_LOAD
	Symbols  *symbol.Table // exported symbol table

	// Adjusted (absolute) addresses of key dynamic tables.
	SymtabAddr  uintptr
	SymtabSize  uint64 // size of symbol table in bytes
	StrtabAddr  uintptr
	StrtabSize  uint64  // size of string table in bytes (from DT_STRSZ)
	HashAddr    uintptr // SysV DT_HASH, 0 if absent
	GnuHashAddr uintptr // DT_GNU_HASH, 0 if absent

	RelaAddr   uintptr
	RelaSize   uint64
	RelaEnt    uint64
	JmpRelAddr uintptr
	JmpRelSize uint64

	InitAddr    uintptr
	FiniAddr    uintptr
	InitArray   uintptr
	InitArraySz uint64
	FiniArray   uintptr
	FiniArraySz uint64

	TLSModule *tls.Module // TLS module, nil if no PT_TLS segment

	// GOT management for code-sequence TLS relocations (TLSGD, TLSLD).
	// Maps symbol index to GOT offset for allocated entries.
	GOTEntries map[uint32]uintptr // symIdx -> offset into GOT
	GOTBase    uintptr            // base address of allocated GOT space
	GOTSize    uintptr            // current size of GOT in bytes
	GOTPages   []uintptr          // additional GOT pages for dynamic expansion

	RefCount int
	Soname   string
}

// elfProt converts ELF segment flags to mmap protection bits.
func elfProt(flags elf.ProgFlag) int {
	prot := mmap.ProtNone
	if flags&elf.PF_R != 0 {
		prot |= mmap.ProtRead
	}
	if flags&elf.PF_W != 0 {
		prot |= mmap.ProtWrite
	}
	if flags&elf.PF_X != 0 {
		prot |= mmap.ProtExec
	}
	return prot
}

// pageDown rounds addr down to a page boundary.
func pageDown(v uint64) uint64 {
	pageSizeMask := uint64(unix.Getpagesize() - 1)
	return v &^ pageSizeMask
}

// pageUp rounds v up to a page boundary.
func pageUp(v uint64) uint64 {
	pageSizeMask := uint64(unix.Getpagesize() - 1)
	return (v + pageSizeMask) &^ pageSizeMask
}

// Load maps the shared object at path into memory and applies relocations.
// resolver is used to look up symbols from already-loaded libraries.
func Load(path string, resolver SymbolResolver) (*Object, error) {
	parsed, err := goelf.Parse(path)
	if err != nil {
		return nil, err
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("loader: open %q: %w", path, err)
	}
	defer f.Close()

	base, err := mmap.MapAnon(uintptr(parsed.MemSize), mmap.ProtNone)
	if err != nil {
		return nil, fmt.Errorf("loader: reserve address space: %w", err)
	}

	obj := &Object{
		Parsed:   parsed,
		Base:     base,
		Symbols:  symbol.NewTable(base),
		RefCount: 1,
	}

	if err := mapSegments(obj, parsed, int(f.Fd())); err != nil {
		_ = mmap.Unmap(base, uintptr(parsed.MemSize))
		return nil, err
	}

	if err := populateObject(obj, parsed); err != nil {
		_ = mmap.Unmap(base, uintptr(parsed.MemSize))
		return nil, err
	}

	if err := finalizeObject(obj, parsed, resolver); err != nil {
		_ = mmap.Unmap(base, uintptr(parsed.MemSize))
		return nil, err
	}

	return obj, nil
}

// mapSegments maps all PT_LOAD segments from the file descriptor into memory.
func mapSegments(obj *Object, parsed *goelf.ParsedObject, fd int) error {
	for _, ph := range parsed.LoadSegments {
		if err := mapSegment(obj, parsed, ph, fd); err != nil {
			return err
		}
	}
	return nil
}

// mapSegment maps a single PT_LOAD segment into memory.
func mapSegment(obj *Object, parsed *goelf.ParsedObject, ph elf.ProgHeader, fd int) error {
	alignedVAddr := pageDown(ph.Vaddr)
	alignedFileOff := pageDown(ph.Off)
	leading := ph.Vaddr - alignedVAddr
	alignedFileSize := pageUp(ph.Filesz + leading)
	alignedMemSize := pageUp(ph.Memsz + leading)

	prot := elfProt(ph.Flags)
	mapAddr := obj.Base + uintptr(alignedVAddr-parsed.BaseVAddr)
	mapProt := computeMapProt(prot, ph.Memsz, ph.Filesz)

	if err := mapFileRegion(mapAddr, alignedFileSize, mapProt, fd, alignedFileOff); err != nil {
		return err
	}
	if err := mapBSSRegion(mapAddr, ph, alignedFileSize, alignedMemSize, prot, mapProt); err != nil {
		return err
	}

	obj.Segments = append(obj.Segments, Segment{
		Addr:     mapAddr,
		Size:     uintptr(alignedMemSize),
		Prot:     prot,
		FileOff:  alignedFileOff,
		MemSize:  ph.Memsz,
		FileSize: ph.Filesz,
	})
	return nil
}

// computeMapProt returns the effective protection flags for mapping.
func computeMapProt(prot int, memsz, filesz uint64) int {
	if memsz > filesz {
		return prot | mmap.ProtWrite
	}
	return prot
}

// mapFileRegion maps the file-backed portion of a segment.
func mapFileRegion(mapAddr uintptr, alignedFileSize uint64, mapProt, fd int, alignedFileOff uint64) error {
	if alignedFileSize == 0 {
		return nil
	}
	_, err := mmap.MapFixed(
		mapAddr, uintptr(alignedFileSize),
		mapProt, mmap.MapPrivate,
		fd, int64(alignedFileOff),
	)
	if err != nil {
		return fmt.Errorf("loader: map segment at 0x%x: %w", mapAddr, err)
	}
	return nil
}

// mapBSSRegion maps the BSS (zero-initialized) portion and restores protection.
func mapBSSRegion(mapAddr uintptr, ph elf.ProgHeader, alignedFileSize, alignedMemSize uint64, prot, mapProt int) error {
	if ph.Memsz <= ph.Filesz {
		return nil
	}

	tailStart := mapAddr + uintptr(alignedFileSize)
	tailSize := uintptr(alignedMemSize) - uintptr(alignedFileSize)
	if tailSize > 0 {
		_, err := mmap.MapFixed(
			tailStart, tailSize,
			mapProt, mmap.MapPrivate|mmap.MapAnonymous,
			-1, 0,
		)
		if err != nil {
			return fmt.Errorf("loader: map BSS at 0x%x: %w", tailStart, err)
		}
	}

	bssStart := mapAddr + uintptr(ph.Vaddr-pageDown(ph.Vaddr)) + uintptr(ph.Filesz)
	pageEnd := mapAddr + uintptr(alignedFileSize)
	if pageEnd > bssStart {
		zeroMem(bssStart, pageEnd-bssStart)
	}

	if prot != mapProt {
		if err := mmap.Protect(mapAddr, uintptr(alignedMemSize), prot); err != nil {
			return fmt.Errorf("loader: mprotect segment: %w", err)
		}
	}
	return nil
}

// populateObject computes dynamic section addresses, loads symbols, and initializes TLS.
// populateDynamicTags populates Object fields from parsed dynamic section entries.
// It converts virtual addresses to absolute addresses using the base address.
func populateDynamicTags(obj *Object, dynTags map[elf.DynTag]uint64, toAbs func(uint64) uintptr) error {
	populateSymbolTags(obj, dynTags, toAbs)
	if err := populateRelocationTags(obj, dynTags, toAbs); err != nil {
		return err
	}
	populateInitFiniTags(obj, dynTags, toAbs)
	populateSoname(obj, dynTags)
	return nil
}

func populateSymbolTags(obj *Object, dynTags map[elf.DynTag]uint64, toAbs func(uint64) uintptr) {
	if v, ok := dynTags[elf.DT_SYMTAB]; ok {
		obj.SymtabAddr = toAbs(v)
	}
	if v, ok := dynTags[elf.DT_STRTAB]; ok {
		obj.StrtabAddr = toAbs(v)
	}
	if v, ok := dynTags[elf.DT_STRSZ]; ok {
		obj.StrtabSize = v
	}
	if v, ok := dynTags[elf.DT_HASH]; ok {
		obj.HashAddr = toAbs(v)
	}
	if v, ok := dynTags[elf.DT_GNU_HASH]; ok {
		obj.GnuHashAddr = toAbs(v)
	}
}

func populateRelocationTags(obj *Object, dynTags map[elf.DynTag]uint64, toAbs func(uint64) uintptr) error {
	if v, ok := dynTags[elf.DT_RELA]; ok {
		obj.RelaAddr = toAbs(v)
	}
	if v, ok := dynTags[elf.DT_RELASZ]; ok {
		obj.RelaSize = v
	}
	if v, ok := dynTags[elf.DT_RELAENT]; ok {
		obj.RelaEnt = v
	}
	if v, ok := dynTags[elf.DT_JMPREL]; ok {
		obj.JmpRelAddr = toAbs(v)
	}
	if v, ok := dynTags[elf.DT_PLTRELSZ]; ok {
		obj.JmpRelSize = v
	}

	// Validate relocation table consistency: if size > 0, address must be non-zero
	if obj.RelaSize > 0 && obj.RelaAddr == 0 {
		return fmt.Errorf("inconsistent relocation table: DT_RELASZ=%d but DT_RELA is missing or zero", obj.RelaSize)
	}
	if obj.JmpRelSize > 0 && obj.JmpRelAddr == 0 {
		return fmt.Errorf("inconsistent PLT relocation table: DT_PLTRELSZ=%d but DT_JMPREL is missing or zero", obj.JmpRelSize)
	}

	return nil
}

func populateInitFiniTags(obj *Object, dynTags map[elf.DynTag]uint64, toAbs func(uint64) uintptr) {
	if v, ok := dynTags[elf.DT_INIT]; ok {
		obj.InitAddr = toAbs(v)
	}
	if v, ok := dynTags[elf.DT_FINI]; ok {
		obj.FiniAddr = toAbs(v)
	}
	if v, ok := dynTags[elf.DT_INIT_ARRAY]; ok {
		obj.InitArray = toAbs(v)
	}
	if v, ok := dynTags[elf.DT_INIT_ARRAYSZ]; ok {
		obj.InitArraySz = v
	}
	if v, ok := dynTags[elf.DT_FINI_ARRAY]; ok {
		obj.FiniArray = toAbs(v)
	}
	if v, ok := dynTags[elf.DT_FINI_ARRAYSZ]; ok {
		obj.FiniArraySz = v
	}
}

func populateSoname(obj *Object, dynTags map[elf.DynTag]uint64) {
	if v, ok := dynTags[elf.DT_SONAME]; ok && obj.StrtabAddr != 0 {
		obj.Soname = symbol.ReadCStringMem(obj.StrtabAddr, uintptr(v), uintptr(obj.StrtabSize))
	}
}

// initializeSymbolTable calculates symbol table size, parses version info, and loads symbols.
func initializeSymbolTable(obj *Object, dynTags map[elf.DynTag]uint64, base uintptr) error {
	var symtabSize uint64
	if syment, ok := dynTags[elf.DT_SYMENT]; ok && syment == 24 {
		if _, ok := dynTags[elf.DT_STRSZ]; ok && obj.SymtabAddr != 0 && obj.StrtabAddr != 0 {
			if obj.StrtabAddr > obj.SymtabAddr {
				symtabSize = uint64(obj.StrtabAddr - obj.SymtabAddr)
			}
		}
	}

	obj.SymtabSize = symtabSize

	symCount := symtabSize / 24
	if symCount > 0 && obj.StrtabAddr != 0 {
		vt, err := symbol.ParseVersionInfo(dynTags, base, obj.StrtabAddr, obj.StrtabSize, symCount)
		if err == nil && vt != nil {
			obj.Symbols.SetVersionTable(vt)
		}
	}

	if obj.SymtabAddr != 0 && obj.StrtabAddr != 0 {
		if err := obj.Symbols.LoadFromDynamic(obj.SymtabAddr, obj.StrtabAddr, symtabSize, obj.StrtabSize); err != nil {
			return fmt.Errorf("symbol table load failed: %w", err)
		}
	}
	return nil
}

// setupTLS registers a TLS module if the object has a PT_TLS segment.
// It locates the TLS initialization data within the mapped segments.
func setupTLS(obj *Object, parsed *goelf.ParsedObject) error {
	if tlsSeg := parsed.TLSSeg; tlsSeg != nil {
		var tlsInitData uintptr
		if tlsSeg.Filesz > 0 {
			for i := range obj.Segments {
				seg := &obj.Segments[i]
				loadSeg := &parsed.LoadSegments[i]
				segStart := parsed.BaseVAddr + loadSeg.Vaddr
				segEnd := segStart + loadSeg.Memsz
				if tlsSeg.Vaddr >= segStart && tlsSeg.Vaddr < segEnd {
					alignedVAddr := pageDown(loadSeg.Vaddr)
					leading := loadSeg.Vaddr - alignedVAddr
					offset := tlsSeg.Vaddr - loadSeg.Vaddr
					tlsInitData = seg.Addr + uintptr(leading) + uintptr(offset)
					break
				}
			}
		}

		tlsModule, err := tls.GlobalManager().RegisterModule(
			tlsSeg.Memsz,
			tlsSeg.Align,
			tlsSeg.Filesz,
			tlsInitData,
		)
		if err != nil {
			return fmt.Errorf("loader: TLS registration failed: %w", err)
		}
		obj.TLSModule = tlsModule
	}
	return nil
}

// populateObject fills in Object fields from parsed ELF data.
// It converts dynamic section entries, initializes symbol tables, and sets up TLS.
func populateObject(obj *Object, parsed *goelf.ParsedObject) error {
	base := obj.Base
	toAbs := func(vaddr uint64) uintptr {
		return base + uintptr(vaddr-parsed.BaseVAddr)
	}

	if err := populateDynamicTags(obj, parsed.DynEntries, toAbs); err != nil {
		return fmt.Errorf("loader: dynamic tags validation failed: %w", err)
	}
	if err := initializeSymbolTable(obj, parsed.DynEntries, base); err != nil {
		return fmt.Errorf("loader: symbol table initialization failed: %w", err)
	}
	return setupTLS(obj, parsed)
}

// finalizeObject applies relocations, RELRO protection, and runs constructors.
func finalizeObject(obj *Object, parsed *goelf.ParsedObject, resolver SymbolResolver) error {
	if err := processRelocations(obj, resolver); err != nil {
		return fmt.Errorf("loader: relocations: %w", err)
	}
	applyRELROProtection(obj, parsed)
	runConstructors(obj)
	return nil
}

// applyRELROProtection applies read-only protection to GNU_RELRO segment.
func applyRELROProtection(obj *Object, parsed *goelf.ParsedObject) {
	if relro := parsed.GNURelroSeg; relro != nil && relro.Vaddr >= parsed.BaseVAddr {
		relroAddr := obj.Base + uintptr(relro.Vaddr-parsed.BaseVAddr)
		relroSize := uintptr(pageUp(relro.Memsz))
		_ = mmap.Protect(relroAddr, relroSize, mmap.ProtRead)
	}
}

// runConstructors executes DT_INIT and DT_INIT_ARRAY initialization functions.
func runConstructors(obj *Object) {
	if obj.InitAddr != 0 {
		callFunc(obj.InitAddr)
	}
	runInitArray(obj)
}

// runInitArray executes DT_INIT_ARRAY initialization functions.
func runInitArray(obj *Object) {
	if obj.InitArray == 0 || obj.InitArraySz == 0 {
		return
	}
	n := obj.InitArraySz / 8
	initArrayPtr := unsafe.Pointer(obj.InitArray)
	for i := uint64(0); i < n; i++ {
		fn := *(*uintptr)(unsafe.Add(initArrayPtr, i*8))
		if fn != 0 {
			fn = adjustFunctionAddr(fn, obj)
			callFunc(fn)
		}
	}
}

// processRelocations applies all RELA and JMPREL relocations in obj.
func processRelocations(obj *Object, resolver SymbolResolver) error {
	if err := applyRelaTable(obj, obj.RelaAddr, obj.RelaSize, resolver); err != nil {
		return err
	}
	if err := applyRelaTable(obj, obj.JmpRelAddr, obj.JmpRelSize, resolver); err != nil {
		return err
	}
	return nil
}

// applyRelative applies a RELATIVE relocation.
func applyRelative(obj *Object, offsetPtr unsafe.Pointer, addend int64) {
	*(*uintptr)(offsetPtr) = obj.Base + uintptr(addend)
}

// apply64 applies a 64-bit absolute relocation.
func apply64(obj *Object, symIdx uint32, offsetPtr unsafe.Pointer, addend int64, resolver SymbolResolver) error {
	S, err := resolveSymForReloc(obj, symIdx, resolver)
	if err != nil {
		return err
	}
	*(*uint64)(offsetPtr) = uint64(S) + uint64(addend)
	return nil
}

// applyGlobDat applies a GLOB_DAT relocation.
func applyGlobDat(obj *Object, symIdx uint32, offsetPtr unsafe.Pointer, resolver SymbolResolver) error {
	S, err := resolveSymForReloc(obj, symIdx, resolver)
	if err != nil {
		return err
	}
	*(*uintptr)(offsetPtr) = S
	return nil
}

// applyJumpSlot applies a JUMP_SLOT relocation.
func applyJumpSlot(obj *Object, symIdx uint32, offsetPtr unsafe.Pointer, resolver SymbolResolver) error {
	S, err := resolveSymForReloc(obj, symIdx, resolver)
	if err != nil {
		return err
	}
	*(*uintptr)(offsetPtr) = S
	return nil
}

// applyCopy applies a COPY relocation.
func applyCopy(obj *Object, symIdx uint32, offsetPtr unsafe.Pointer, resolver SymbolResolver) error {
	S, err := resolveSymForReloc(obj, symIdx, resolver)
	if err != nil {
		return err
	}
	// Size must come from the source symbol (the exporting library).
	// We look it up by name; a missing entry is an error, not a
	// silent 8-byte fallback, to prevent silent memory corruption.
	name := symName(obj, symIdx)
	sym, ok := obj.Symbols.Lookup(name)
	if !ok {
		return fmt.Errorf("relocCopy: symbol %q not found in symbol table", name)
	}
	dst := unsafe.Slice((*byte)(offsetPtr), sym.Size)
	src := unsafe.Slice((*byte)(unsafe.Pointer(S)), sym.Size)
	copy(dst, src)
	return nil
}

// apply32 applies a 32-bit absolute relocation.
func apply32(obj *Object, symIdx uint32, offsetPtr unsafe.Pointer, addend int64, resolver SymbolResolver) error {
	S, err := resolveSymForReloc(obj, symIdx, resolver)
	if err != nil {
		return err
	}
	*(*uint32)(offsetPtr) = uint32(uint64(S) + uint64(addend))
	return nil
}

// apply32S applies a 32-bit signed relocation.
func apply32S(obj *Object, symIdx uint32, offsetPtr unsafe.Pointer, addend int64, resolver SymbolResolver) error {
	S, err := resolveSymForReloc(obj, symIdx, resolver)
	if err != nil {
		return err
	}
	*(*int32)(offsetPtr) = int32(int64(S) + addend)
	return nil
}

// applyPC32 applies a PC-relative 32-bit relocation (PC32 or PLT32).
func applyPC32(obj *Object, symIdx uint32, offsetPtr unsafe.Pointer, addend int64, offset uintptr, resolver SymbolResolver) error {
	S, err := resolveSymForReloc(obj, symIdx, resolver)
	if err != nil {
		return err
	}
	*(*uint32)(offsetPtr) = uint32(int64(S) + addend - int64(offset))
	return nil
}

// applyDTPMod64 applies a DTPMOD64 TLS relocation (module ID).
func applyDTPMod64(obj *Object, r *relaEntry, offsetPtr unsafe.Pointer) error {
	if obj.TLSModule == nil {
		return fmt.Errorf("relocDTPMod64 relocation at %#x but library has no PT_TLS segment", r.Offset)
	}
	*(*uint64)(offsetPtr) = obj.TLSModule.GetModuleID()
	return nil
}

// applyDTPOff64 applies a DTPOFF64 TLS relocation (module-relative offset).
func applyDTPOff64(obj *Object, symIdx uint32, r *relaEntry, offsetPtr unsafe.Pointer, addend int64, resolver SymbolResolver) error {
	if obj.TLSModule == nil {
		return fmt.Errorf("relocDTPOff64 relocation at %#x but library has no PT_TLS segment", r.Offset)
	}
	// For DTPOFF, we use the symbol value (if present) + addend.
	// If no symbol, just use addend as the offset.
	var symValue uint64
	if symIdx != 0 {
		S, err := resolveSymForReloc(obj, symIdx, resolver)
		if err != nil {
			// For TLS, if symbol not found, it might be module-local.
			// Use the symbol from our own symbol table if available.
			name := symName(obj, symIdx)
			if name != "" {
				if sym, ok := obj.Symbols.Lookup(name); ok {
					// Symbol value is relative to module base.
					// For TLS symbols, st_value is the offset within the TLS block.
					symValue = uint64(sym.Value - obj.Base)
				} else {
					return fmt.Errorf("relocDTPOff64: TLS symbol %q not found", name)
				}
			}
		} else {
			symValue = uint64(S - obj.Base)
		}
	}
	*(*int64)(offsetPtr) = int64(symValue) + addend
	return nil
}

// applyTPOff64 applies a TPOFF64 TLS relocation (thread pointer relative offset).
func applyTPOff64(obj *Object, symIdx uint32, r *relaEntry, offsetPtr unsafe.Pointer, addend int64, resolver SymbolResolver) error {
	// For libraries without PT_TLS that reference external TLS symbols (like libm
	// referencing libc's errno), we need to resolve the symbol from the providing library.
	var tlsModule *tls.Module
	var symOffsetInModule int64

	if symIdx != 0 {
		name := symName(obj, symIdx)
		if name == "" {
			// No symbol name, use current library's TLS module if available.
			if obj.TLSModule != nil {
				tlsModule = obj.TLSModule
			} else {
				*(*int64)(offsetPtr) = 0
				return nil
			}
		} else {
			// Try to resolve the symbol and find which library provides it.
			addr, providerObj, err := resolver.ResolveWithLibrary(name)
			if err != nil {
				// Symbol not found - for weak symbols, write 0.
				bind := symBind(obj, symIdx)
				if bind == 2 { // STB_WEAK
					*(*int64)(offsetPtr) = 0
					return nil
				}
				// For strong symbols, write 0 to avoid crashes but this is an error state.
				*(*int64)(offsetPtr) = 0
				return nil
			}

			// Determine which TLS module to use based on the providing library.
			if providerObj != nil && providerObj.TLSModule != nil {
				// External symbol from a library with TLS - use that library's module.
				tlsModule = providerObj.TLSModule
				symOffsetInModule = int64(addr - providerObj.Base)
			} else if obj.TLSModule != nil {
				// Symbol resolved but provider has no TLS, or it's our own symbol - use our module.
				tlsModule = obj.TLSModule
				symOffsetInModule = int64(addr - obj.Base)
			} else {
				// Neither provider nor current library has TLS - write 0.
				*(*int64)(offsetPtr) = 0
				return nil
			}
		}
	} else if obj.TLSModule != nil {
		tlsModule = obj.TLSModule
	} else {
		// No symbol and no TLS module - write 0.
		*(*int64)(offsetPtr) = 0
		return nil
	}

	// Allocate a TLS block and compute the offset.
	block, err := tls.GlobalManager().AllocateBlock(tlsModule)
	if err != nil {
		return fmt.Errorf("relocTPOff64: failed to allocate TLS block: %w", err)
	}
	// The thread pointer offset is negative from the TP.
	offset := block.GetThreadPointerOffset()
	offset += symOffsetInModule
	*(*int64)(offsetPtr) = offset + addend
	return nil
}

// applyDTPOff32 applies a DTPOFF32 TLS relocation (32-bit module-relative offset).
func applyDTPOff32(obj *Object, symIdx uint32, r *relaEntry, offsetPtr unsafe.Pointer, addend int64, resolver SymbolResolver) error {
	if obj.TLSModule == nil {
		return fmt.Errorf("relocDTPOff32 relocation at %#x but library has no PT_TLS segment", r.Offset)
	}
	var symValue uint64
	if symIdx != 0 {
		S, err := resolveSymForReloc(obj, symIdx, resolver)
		if err != nil {
			name := symName(obj, symIdx)
			if name != "" {
				if sym, ok := obj.Symbols.Lookup(name); ok {
					symValue = uint64(sym.Value - obj.Base)
				} else {
					return fmt.Errorf("relocDTPOff32: TLS symbol %q not found", name)
				}
			}
		} else {
			symValue = uint64(S - obj.Base)
		}
	}
	*(*int32)(offsetPtr) = int32(int64(symValue) + addend)
	return nil
}

// applyTPOff32 applies a TPOFF32 TLS relocation (32-bit thread pointer relative offset).
func applyTPOff32(obj *Object, symIdx uint32, r *relaEntry, offsetPtr unsafe.Pointer, addend int64, resolver SymbolResolver) error {
	if obj.TLSModule == nil {
		return fmt.Errorf("relocTPOff32 relocation at %#x but library has no PT_TLS segment", r.Offset)
	}
	block, err := tls.GlobalManager().AllocateBlock(obj.TLSModule)
	if err != nil {
		return fmt.Errorf("relocTPOff32: failed to allocate TLS block: %w", err)
	}
	offset := block.GetThreadPointerOffset()
	if symIdx != 0 {
		S, err := resolveSymForReloc(obj, symIdx, resolver)
		if err == nil {
			offset += int64(S - obj.Base)
		}
	}
	*(*int32)(offsetPtr) = int32(offset + addend)
	return nil
}

// applyIRelative applies an IRELATIVE relocation (IFUNC resolver).
func applyIRelative(obj *Object, offsetPtr unsafe.Pointer, addend int64) {
	// For relocIRelative, the addend points to the resolver function.
	// We call it to get the actual function address.
	resolverAddr := obj.Base + uintptr(addend)
	resolvedAddr := CallIfuncResolver(resolverAddr)
	*(*uintptr)(offsetPtr) = resolvedAddr
}

// relocContext contains the pre-computed values needed for relocation processing.
type relocContext struct {
	obj       *Object
	r         *relaEntry
	symIdx    uint32
	offset    uintptr
	offsetPtr unsafe.Pointer
	addend    int64
	resolver  SymbolResolver
}

// relocHandler processes a specific type of relocation.
type relocHandler func(ctx *relocContext) error

// relocHandlers maps relocation types to their handler functions.
var relocHandlers = map[uint32]relocHandler{
	relocNone: func(ctx *relocContext) error {
		return nil
	},
	relocRelative: func(ctx *relocContext) error {
		applyRelative(ctx.obj, ctx.offsetPtr, ctx.addend)
		return nil
	},
	reloc64: func(ctx *relocContext) error {
		return apply64(ctx.obj, ctx.symIdx, ctx.offsetPtr, ctx.addend, ctx.resolver)
	},
	relocGlobDat: func(ctx *relocContext) error {
		return applyGlobDat(ctx.obj, ctx.symIdx, ctx.offsetPtr, ctx.resolver)
	},
	relocJumpSlot: func(ctx *relocContext) error {
		return applyJumpSlot(ctx.obj, ctx.symIdx, ctx.offsetPtr, ctx.resolver)
	},
	relocCopy: func(ctx *relocContext) error {
		return applyCopy(ctx.obj, ctx.symIdx, ctx.offsetPtr, ctx.resolver)
	},
	reloc32: func(ctx *relocContext) error {
		return apply32(ctx.obj, ctx.symIdx, ctx.offsetPtr, ctx.addend, ctx.resolver)
	},
	reloc32S: func(ctx *relocContext) error {
		return apply32S(ctx.obj, ctx.symIdx, ctx.offsetPtr, ctx.addend, ctx.resolver)
	},
	relocPC32: func(ctx *relocContext) error {
		return applyPC32(ctx.obj, ctx.symIdx, ctx.offsetPtr, ctx.addend, ctx.offset, ctx.resolver)
	},
	relocPLT32: func(ctx *relocContext) error {
		return applyPC32(ctx.obj, ctx.symIdx, ctx.offsetPtr, ctx.addend, ctx.offset, ctx.resolver)
	},
	relocDTPMod64: func(ctx *relocContext) error {
		return applyDTPMod64(ctx.obj, ctx.r, ctx.offsetPtr)
	},
	relocDTPOff64: func(ctx *relocContext) error {
		return applyDTPOff64(ctx.obj, ctx.symIdx, ctx.r, ctx.offsetPtr, ctx.addend, ctx.resolver)
	},
	relocTPOff64: func(ctx *relocContext) error {
		return applyTPOff64(ctx.obj, ctx.symIdx, ctx.r, ctx.offsetPtr, ctx.addend, ctx.resolver)
	},
	relocDTPOff32: func(ctx *relocContext) error {
		return applyDTPOff32(ctx.obj, ctx.symIdx, ctx.r, ctx.offsetPtr, ctx.addend, ctx.resolver)
	},
	relocTPOff32: func(ctx *relocContext) error {
		return applyTPOff32(ctx.obj, ctx.symIdx, ctx.r, ctx.offsetPtr, ctx.addend, ctx.resolver)
	},
	relocGOTTPOff: func(ctx *relocContext) error {
		return applyGottpoff(ctx.obj, ctx.r, ctx.symIdx, ctx.offsetPtr, ctx.addend, ctx.resolver)
	},
	relocTLSGD: func(ctx *relocContext) error {
		return applyTlsgd(ctx.obj, ctx.r, ctx.resolver)
	},
	relocTLSLD: func(ctx *relocContext) error {
		return applyTlsld(ctx.obj, ctx.r)
	},
	relocIRelative: func(ctx *relocContext) error {
		applyIRelative(ctx.obj, ctx.offsetPtr, ctx.addend)
		return nil
	},
}

func applyRelaTable(obj *Object, tableAddr uintptr, tableSize uint64, resolver SymbolResolver) error {
	if tableAddr == 0 || tableSize == 0 {
		return nil
	}

	// Validate that relocation table size is properly aligned to entry size
	if tableSize%relaEntSize != 0 {
		return fmt.Errorf("relocation table size %d is not aligned to entry size %d", tableSize, relaEntSize)
	}

	n := tableSize / relaEntSize
	rels := unsafe.Slice((*relaEntry)(unsafe.Pointer(tableAddr)), n)

	for i := uint64(0); i < n; i++ {
		r := &rels[i]
		// Validate relocation offset is within mapped memory range
		maxOffset := obj.Parsed.BaseVAddr + obj.Parsed.MemSize
		if r.Offset < obj.Parsed.BaseVAddr || r.Offset >= maxOffset {
			return fmt.Errorf("relocation offset %#x out of range [%#x, %#x)", r.Offset, obj.Parsed.BaseVAddr, maxOffset)
		}

		ctx := &relocContext{
			obj:      obj,
			r:        r,
			symIdx:   relaSymIdx(r.Info),
			offset:   obj.Base + uintptr(r.Offset-obj.Parsed.BaseVAddr),
			addend:   r.Addend,
			resolver: resolver,
		}
		ctx.offsetPtr = unsafe.Pointer(ctx.offset)

		relocType := relaType(r.Info)
		handler, ok := relocHandlers[relocType]
		if !ok {
			return fmt.Errorf("unknown relocation type %d at offset %#x", relocType, r.Offset)
		}

		if err := handler(ctx); err != nil {
			return err
		}
	}
	return nil
}

// applyGottpoff handles relocGOTTPOff relocations (PC-relative GOT reference for Initial Exec TLS).
func applyGottpoff(obj *Object, r *relaEntry, symIdx uint32, offsetPtr unsafe.Pointer, addend int64, resolver SymbolResolver) error {
	// PC-relative GOT reference for Initial Exec TLS model.
	// Code: movq x@gottpoff(%rip), %reg
	// The relocation patches a 32-bit PC-relative offset in the instruction.
	// The offset points to a GOT entry containing the TP offset for the symbol.
	if obj.TLSModule == nil {
		return fmt.Errorf("relocGOTTPOff relocation at %#x but library has no PT_TLS segment", r.Offset)
	}
	// For simplicity, we treat this as a PC-relative offset calculation.
	// The symbol should point to a GOT entry that will be/is populated with TP offset.
	var gotEntry uintptr
	if symIdx != 0 {
		// Resolve the symbol to get the GOT entry address.
		S, err := resolveSymForReloc(obj, symIdx, resolver)
		if err == nil {
			gotEntry = S
		} else {
			// If symbol resolution fails, this might be a local TLS symbol.
			// Fall back to computing TP offset directly.
			block, err := tls.GlobalManager().AllocateBlock(obj.TLSModule)
			if err != nil {
				return fmt.Errorf("relocGOTTPOff: failed to allocate TLS block: %w", err)
			}
			// Write TP offset directly as the "GOT entry value".
			// This is a simplification that works when the GOT entry is co-located.
			tpOff := block.GetThreadPointerOffset()
			*(*int32)(offsetPtr) = int32(tpOff + addend)
			return nil
		}
	}
	// Compute PC-relative offset: gotEntry - (reloc_site + 4)
	relocSite := obj.Base + uintptr(r.Offset)
	pcRelOffset := int64(gotEntry) - int64(relocSite) - 4
	*(*int32)(offsetPtr) = int32(pcRelOffset + addend)
	return nil
}

// allocateGOTEntryPair allocates a GOT entry pair (16 bytes) for a TLS symbol.
// Returns the address of the first entry (DTPMOD64 entry).
// The second entry (DTPOFF64) is at returned_address + 8.
// Dynamically allocates additional GOT pages when needed.
func allocateGOTEntryPair(obj *Object, symIdx uint32) (uintptr, error) {
	// Check if already allocated.
	if offset, exists := obj.GOTEntries[symIdx]; exists {
		return obj.GOTBase + offset, nil
	}

	// Lazy-allocate GOT space on first use.
	// We allocate a page of writable memory to hold GOT entries.
	if obj.GOTBase == 0 {
		const gotPageSize = 4096 // One page for GOT entries
		addr, err := mmap.MapAnon(gotPageSize, mmap.ProtRead|mmap.ProtWrite)
		if err != nil {
			return 0, fmt.Errorf("failed to allocate GOT space: %w", err)
		}
		obj.GOTBase = addr
		obj.GOTEntries = make(map[uint32]uintptr)
		obj.GOTPages = []uintptr{addr} // Track the initial page
	}

	// Calculate current page index and offset within page.
	const gotPageSize = 4096
	requiredSize := obj.GOTSize + 16

	// Check if we need to allocate a new page.
	totalAllocated := uintptr(len(obj.GOTPages)) * gotPageSize
	if requiredSize > totalAllocated {
		newAddr, err := mmap.MapAnon(gotPageSize, mmap.ProtRead|mmap.ProtWrite)
		if err != nil {
			return 0, fmt.Errorf("failed to allocate additional GOT page: %w", err)
		}
		obj.GOTPages = append(obj.GOTPages, newAddr)
	}

	// Allocate 16 bytes (two uint64 entries) at current GOTSize offset.
	offset := obj.GOTSize
	obj.GOTEntries[symIdx] = offset
	obj.GOTSize += 16

	// Compute the actual address: base + offset for first page,
	// or corresponding address in additional pages.
	pageIndex := offset / gotPageSize
	pageOffset := offset % gotPageSize
	entryAddr := obj.GOTPages[pageIndex] + pageOffset

	return entryAddr, nil
}

// applyTlsgd handles relocTLSGD relocations (General Dynamic TLS model).
func applyTlsgd(obj *Object, r *relaEntry, resolver SymbolResolver) error {
	// PC-relative reference to GOT entries for General Dynamic TLS model.
	// Code: leaq x@tlsgd(%rip), %rdi; call __tls_get_addr
	// The relocation patches the leaq instruction's PC-relative offset
	// to point to a pair of GOT entries containing [DTPMOD64, DTPOFF64].
	if obj.TLSModule == nil {
		return fmt.Errorf("relocTLSGD relocation at %#x but library has no PT_TLS segment", r.Offset)
	}

	symIdx := uint32(r.Info >> 32)

	// Allocate or retrieve GOT entry pair for this symbol.
	gotEntry, err := allocateGOTEntryPair(obj, symIdx)
	if err != nil {
		return fmt.Errorf("relocTLSGD at %#x: %w", r.Offset, err)
	}

	// Populate GOT entries: [DTPMOD64, DTPOFF64].
	// DTPMOD64 = module ID
	moduleID := obj.TLSModule.GetModuleID()
	*(*uint64)(unsafe.Pointer(gotEntry)) = moduleID

	// DTPOFF64 = symbol offset within TLS block
	// For TLS symbols, st_value contains the offset within the PT_TLS segment.
	// We need to resolve the symbol to get its TLS offset.
	tlsOffset := uint64(r.Addend) // Base offset from addend
	if symIdx != 0 {
		// Get symbol value (TLS offset within PT_TLS)
		symAddr := symAddress(obj, symIdx)
		// For TLS symbols, st_value is already the offset, not an address
		tlsOffset = uint64(int64(symAddr) + r.Addend)
	}
	*(*uint64)(unsafe.Pointer(gotEntry + 8)) = tlsOffset

	// Compute PC-relative offset from relocation site to GOT entry.
	// The instruction is: leaq symbol@tlsgd(%rip), %rdi
	// We patch the 32-bit PC-relative offset in the instruction.
	relocSite := obj.Base + uintptr(r.Offset)
	pcRelOffset := int64(gotEntry) - int64(relocSite+4) // +4 for instruction size

	// Write the PC-relative offset to the relocation site.
	offsetPtr := unsafe.Pointer(relocSite)
	*(*int32)(offsetPtr) = int32(pcRelOffset)

	return nil
}

// applyTlsld handles relocTLSLD relocations (Local Dynamic TLS model).
func applyTlsld(obj *Object, r *relaEntry) error {
	// PC-relative reference to GOT entries for Local Dynamic TLS model.
	// Code: leaq symbol@tlsld(%rip), %rdi; call __tls_get_addr
	// The relocation patches the leaq instruction's PC-relative offset
	// to point to a pair of GOT entries containing [DTPMOD64, 0].
	// Unlike TLSGD, TLSLD doesn't reference a specific symbol - it gets
	// the base address of the module's TLS block.
	if obj.TLSModule == nil {
		return fmt.Errorf("relocTLSLD relocation at %#x but library has no PT_TLS segment", r.Offset)
	}

	// For TLSLD, symIdx is typically 0 (no specific symbol).
	// Use a special marker (0xFFFFFFFF) to avoid collision with real symbol indices.
	const tlsldMarker = uint32(0xFFFFFFFF)

	// Allocate or retrieve GOT entry pair for TLSLD.
	gotEntry, err := allocateGOTEntryPair(obj, tlsldMarker)
	if err != nil {
		return fmt.Errorf("relocTLSLD at %#x: %w", r.Offset, err)
	}

	// Populate GOT entries: [DTPMOD64, 0].
	// DTPMOD64 = module ID
	moduleID := obj.TLSModule.GetModuleID()
	*(*uint64)(unsafe.Pointer(gotEntry)) = moduleID

	// Second entry is 0 (no symbol-specific offset for Local Dynamic)
	*(*uint64)(unsafe.Pointer(gotEntry + 8)) = 0

	// Compute PC-relative offset from relocation site to GOT entry.
	// The instruction is: leaq symbol@tlsld(%rip), %rdi
	// We patch the 32-bit PC-relative offset in the instruction.
	relocSite := obj.Base + uintptr(r.Offset)
	pcRelOffset := int64(gotEntry) - int64(relocSite+4) // +4 for instruction size

	// Write the PC-relative offset to the relocation site.
	offsetPtr := unsafe.Pointer(relocSite)
	*(*int32)(offsetPtr) = int32(pcRelOffset)

	return nil
}

// resolveSymForReloc returns the absolute address of the symbol at symIdx.
// If symIdx is 0, returns 0 (used for relocRelative which has no symbol).
// Weak symbols (STB_WEAK) resolve to 0 if not found, per ELF specification.
func resolveSymForReloc(obj *Object, symIdx uint32, resolver SymbolResolver) (uintptr, error) {
	if symIdx == 0 {
		return 0, nil
	}
	name := symName(obj, symIdx)
	if name == "" {
		return 0, nil
	}

	// Try our own symbol table first.
	if sym, ok := obj.Symbols.Lookup(name); ok {
		return sym.Value, nil
	}

	// Fall back to the external resolver.
	if resolver != nil {
		addr, err := resolver.Resolve(name)
		if err == nil {
			return addr, nil
		}
	}

	// Weak symbols are allowed to be unresolved and resolve to NULL.
	bind := symBind(obj, symIdx)
	if bind == 2 { // STB_WEAK = 2
		return 0, nil
	}

	return 0, fmt.Errorf("undefined symbol: %q", name)
}

// Unload runs destructors and unmaps all segments of obj.
func Unload(obj *Object) error {
	runFiniCallbacks(obj)
	if err := unmapGOTPages(obj); err != nil {
		return err
	}
	return mmap.Unmap(obj.Base, uintptr(obj.Parsed.MemSize))
}

// runFiniCallbacks executes DT_FINI_ARRAY and DT_FINI destructors.
func runFiniCallbacks(obj *Object) {
	runFiniArray(obj)
	runFiniFunction(obj)
}

// runFiniArray executes DT_FINI_ARRAY in reverse order with panic recovery.
func runFiniArray(obj *Object) {
	if obj.FiniArray == 0 || obj.FiniArraySz == 0 {
		return
	}
	n := obj.FiniArraySz / 8
	finiArrayPtr := unsafe.Pointer(obj.FiniArray)
	for i := n; i > 0; i-- {
		fn := *(*uintptr)(unsafe.Add(finiArrayPtr, (i-1)*8))
		if fn != 0 {
			fn = adjustFunctionAddr(fn, obj)
			callFuncSafe(fn)
		}
	}
}

// runFiniFunction executes DT_FINI with panic recovery.
func runFiniFunction(obj *Object) {
	if obj.FiniAddr != 0 {
		callFuncSafe(obj.FiniAddr)
	}
}

// adjustFunctionAddr converts virtual addresses to absolute addresses.
func adjustFunctionAddr(fn uintptr, obj *Object) uintptr {
	if fn < uintptr(obj.Parsed.MemSize) {
		return obj.Base + fn
	}
	return fn
}

// callFuncSafe calls fn with panic recovery to allow cleanup to continue.
func callFuncSafe(fn uintptr) {
	defer func() {
		if r := recover(); r != nil {
			_ = r // Ignore panics during cleanup
		}
	}()
	callFunc(fn)
}

// unmapGOTPages unmaps all dynamically allocated GOT pages.
func unmapGOTPages(obj *Object) error {
	if len(obj.GOTPages) == 0 {
		return nil
	}
	const gotPageSize = 4096
	for _, pageAddr := range obj.GOTPages {
		if err := mmap.Unmap(pageAddr, gotPageSize); err != nil {
			return fmt.Errorf("failed to unmap GOT page at %#x: %w", pageAddr, err)
		}
	}
	return nil
}

// zeroMem zeroes count bytes starting at addr.
//
//go:nocheckptr
func zeroMem(addr, count uintptr) {
	sl := unsafe.Slice((*byte)(unsafe.Pointer(addr)), count)
	for i := range sl {
		sl[i] = 0
	}
}
