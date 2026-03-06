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

// LoadFromDynamic reads Elf64_Sym entries from the in-memory symbol table and
// populates t. symtabSize is the total byte size of the symbol table.
func (t *Table) LoadFromDynamic(symtabAddr, strtabAddr uintptr, symtabSize uint64) error {
	if symtabAddr == 0 {
		return fmt.Errorf("symbol: symtabAddr is 0")
	}
	if symtabSize == 0 {
		// DT_SYMENT tells the entry size but DT_STRSZ is for the string table.
		// Without a reliable size we do a best-effort scan until we hit a null
		// entry; 4096 symbols is a sane upper bound for this fallback.
		symtabSize = symEntSize * 4096
	}

	n := symtabSize / symEntSize
	syms := unsafe.Slice((*Elf64Sym)(unsafe.Pointer(symtabAddr)), n)

	for i := uint64(0); i < n; i++ {
		s := &syms[i]
		bind := elf.SymBind(s.Info >> 4)
		symType := elf.SymType(s.Info & 0xf)

		if s.Name == 0 && s.Value == 0 {
			continue // null/empty entry
		}
		if bind != elf.STB_GLOBAL && bind != elf.STB_WEAK {
			continue
		}
		if s.Shndx == uint16(elf.SHN_UNDEF) {
			continue // undefined; resolved from elsewhere
		}

		name := ReadCStringMem(strtabAddr, uintptr(s.Name))
		if name == "" {
			continue
		}

		sym := &Symbol{
			Name:    name,
			Value:   t.base + uintptr(s.Value),
			Size:    s.Size,
			Bind:    bind,
			Type:    symType,
			Section: elf.SectionIndex(s.Shndx),
		}

		// Add version information if available.
		if t.versions != nil {
			verIdx := t.versions.GetSymbolVersion(uint32(i))
			sym.VerIdx = verIdx
			if verIdx > 1 { // 0=local, 1=global/default, >1=specific version
				sym.VerName = t.versions.GetVersionName(verIdx)
			}
		}

		t.symbols[name] = sym
	}
	return nil
}

// ReadCStringMem reads a null-terminated C string from memory at base+offset.
func ReadCStringMem(base, offset uintptr) string {
	basePtr := unsafe.Pointer(base)
	ptr := unsafe.Add(basePtr, offset)
	var buf []byte
	for {
		b := *(*byte)(ptr)
		if b == 0 {
			break
		}
		buf = append(buf, b)
		ptr = unsafe.Add(ptr, 1)
	}
	return string(buf)
}
