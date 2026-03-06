package symbol

import (
	"debug/elf"
	"fmt"
	"unsafe"
)

// Symbol represents a single ELF symbol after loading.
type Symbol struct {
	Name    string
	Value   uintptr // absolute address (base + st_value)
	Size    uint64
	Bind    elf.SymBind
	Type    elf.SymType
	Section elf.SectionIndex // mirrors st_shndx; use elf.SHN_* constants
	VerIdx  uint16           // version index from DT_VERSYM
	VerName string           // version name (e.g., "GLIBC_2.2.5")
}

// Table is a name-to-Symbol index for a loaded shared object.
type Table struct {
	symbols  map[string]*Symbol
	base     uintptr
	versions *VersionTable // symbol version information (may be nil)
}

// NewTable creates an empty Table with the given load base.
func NewTable(base uintptr) *Table {
	return &Table{
		symbols:  make(map[string]*Symbol),
		base:     base,
		versions: nil,
	}
}

// SetVersionTable assigns version information to this symbol table.
func (t *Table) SetVersionTable(vt *VersionTable) {
	t.versions = vt
}

// AddSymbol inserts or replaces a symbol in the table.
func (t *Table) AddSymbol(name string, s *Symbol) {
	t.symbols[name] = s
}

// Lookup returns the Symbol with the given name, if present.
// If multiple versions exist, returns the default version.
func (t *Table) Lookup(name string) (*Symbol, bool) {
	s, ok := t.symbols[name]
	return s, ok
}

// LookupVersion returns the Symbol with the given name and version.
// If version is empty, behaves like Lookup (returns default version).
// Version string should be like "GLIBC_2.2.5" or empty for unversioned.
func (t *Table) LookupVersion(name, version string) (*Symbol, bool) {
	if version == "" {
		return t.Lookup(name)
	}
	// For now, we only support exact name matches since we store one symbol per name.
	// In a full implementation, we'd maintain name@version keys.
	// This is sufficient for the common case where the library provides one version.
	s, ok := t.symbols[name]
	if !ok {
		return nil, false
	}
	// If version matches or symbol is unversioned, return it.
	if s.VerName == version || s.VerName == "" {
		return s, true
	}
	return nil, false
}

// ForEach calls fn for every symbol in the table.
func (t *Table) ForEach(fn func(name string, s *Symbol)) {
	for k, v := range t.symbols {
		fn(k, v)
	}
}

// Elf64Sym mirrors the on-wire layout of Elf64_Sym (24 bytes, little-endian).
//
//	st_name  uint32  // 0..3
//	st_info  uint8   // 4
//	st_other uint8   // 5
//	st_shndx uint16  // 6..7
//	st_value uint64  // 8..15
//	st_size  uint64  // 16..23
type Elf64Sym struct {
	Name  uint32
	Info  uint8
	Other uint8
	Shndx uint16
	Value uint64
	Size  uint64
}

const symEntSize = 24

// maxFallbackSymbols is the upper bound for symbol table size when DT_HASH/DT_GNU_HASH
// is absent and we cannot determine the actual size. 4096 symbols (98,304 bytes) handles:
//   - Small libraries: glibc (~2000 symbols), libm (~500), libpthread (~400)
//   - Most application libraries: <1000 symbols typical
//   - Early termination: loop stops at first all-zero entry (name=0, value=0)
//
// This may be insufficient for mega-libraries (libLLVM.so ~50K symbols, libQt5Core.so ~10K),
// but those cases are rare and typically provide DT_HASH or DT_GNU_HASH for size computation.
const maxFallbackSymbols = 4096

// LoadFromDynamic reads Elf64_Sym entries from the in-memory symbol table and
// populates t. symtabSize is the total byte size of the symbol table.
// strtabSize is the size of the string table in bytes (for bounds checking).
func (t *Table) LoadFromDynamic(symtabAddr, strtabAddr uintptr, symtabSize, strtabSize uint64) error {
	if symtabAddr == 0 {
		return fmt.Errorf("symbol: symtabAddr is 0")
	}
	symtabSize = computeSymtabSize(symtabSize)
	n := symtabSize / symEntSize

	for i := uint64(0); i < n; i++ {
		if err := t.loadSymbolEntry(symtabAddr, strtabAddr, i, strtabSize); err != nil {
			return err
		}
	}
	return nil
}

// computeSymtabSize returns the symbol table size, using a fallback if unknown.
func computeSymtabSize(symtabSize uint64) uint64 {
	if symtabSize == 0 {
		return symEntSize * maxFallbackSymbols
	}
	return symtabSize
}

// loadSymbolEntry processes a single symbol table entry.
func (t *Table) loadSymbolEntry(symtabAddr, strtabAddr uintptr, idx, strtabSize uint64) error {
	symPtr := unsafe.Add(unsafe.Pointer(symtabAddr), idx*symEntSize)
	s := (*Elf64Sym)(symPtr)

	if !shouldProcessSymbol(s) {
		return nil
	}

	name := ReadCStringMem(strtabAddr, uintptr(s.Name), uintptr(strtabSize))
	if name == "" {
		return nil
	}

	sym := t.buildSymbol(s, name, idx)
	if t.shouldSkipSymbol(name, idx) {
		return nil
	}

	t.symbols[name] = sym
	return nil
}

// shouldProcessSymbol checks if a symbol entry should be processed.
func shouldProcessSymbol(s *Elf64Sym) bool {
	if s.Name == 0 && s.Value == 0 {
		return false // null/empty entry
	}
	bind := elf.SymBind(s.Info >> 4)
	if bind != elf.STB_GLOBAL && bind != elf.STB_WEAK {
		return false
	}
	if s.Shndx == uint16(elf.SHN_UNDEF) {
		return false // undefined; resolved from elsewhere
	}
	return true
}

// buildSymbol constructs a Symbol from an Elf64Sym entry.
func (t *Table) buildSymbol(s *Elf64Sym, name string, idx uint64) *Symbol {
	bind := elf.SymBind(s.Info >> 4)
	symType := elf.SymType(s.Info & 0xf)

	sym := &Symbol{
		Name:    name,
		Value:   t.base + uintptr(s.Value),
		Size:    s.Size,
		Bind:    bind,
		Type:    symType,
		Section: elf.SectionIndex(s.Shndx),
	}

	if t.versions != nil {
		verIdx := t.versions.GetSymbolVersion(uint32(idx))
		sym.VerIdx = verIdx
		if verIdx > 1 {
			sym.VerName = t.versions.GetVersionName(verIdx)
		}
	}

	return sym
}

// shouldSkipSymbol determines if a symbol should be skipped during loading
// when multiple versions of the same symbol exist. Returns true if this symbol
// should be skipped in favor of an existing one.
//
// Symbol version priority:
//   - Hidden versions (marked with 0x8000 bit in DT_VERSYM) are skipped if a symbol
//     with the same name already exists (prefer default @@ over non-default @)
//   - Non-hidden versions always replace existing symbols
func (t *Table) shouldSkipSymbol(name string, symIdx uint64) bool {
	// If no existing symbol, don't skip
	if _, exists := t.symbols[name]; !exists {
		return false
	}

	// Check if current symbol is hidden (non-default version)
	if t.versions != nil && t.versions.SymbolVersions != nil && symIdx < uint64(len(t.versions.SymbolVersions)) {
		isHidden := (t.versions.SymbolVersions[symIdx] & 0x8000) != 0
		if isHidden {
			return true // Skip hidden version, keep existing
		}
	}

	return false // Non-hidden version replaces existing
}

// ReadCStringMem reads a null-terminated C string from memory at base+offset.
// The limit parameter specifies the maximum valid offset (size of the string table).
// If limit is 0, no bounds checking is performed (for backward compatibility).
// Returns empty string if offset is out of bounds or no null terminator is found within limit.
func ReadCStringMem(base, offset, limit uintptr) string {
	if limit > 0 && offset >= limit {
		return ""
	}
	basePtr := unsafe.Pointer(base)
	ptr := unsafe.Add(basePtr, offset)
	var buf []byte
	bytesRead := uintptr(0)
	for {
		if limit > 0 && offset+bytesRead >= limit {
			return ""
		}
		b := *(*byte)(ptr)
		if b == 0 {
			break
		}
		buf = append(buf, b)
		ptr = unsafe.Add(ptr, 1)
		bytesRead++
	}
	return string(buf)
}
