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
}

// Table is a name-to-Symbol index for a loaded shared object.
type Table struct {
	symbols map[string]*Symbol
	base    uintptr
}

// NewTable creates an empty Table with the given load base.
func NewTable(base uintptr) *Table {
	return &Table{
		symbols: make(map[string]*Symbol),
		base:    base,
	}
}

// AddSymbol inserts or replaces a symbol in the table.
func (t *Table) AddSymbol(name string, s *Symbol) {
	t.symbols[name] = s
}

// Lookup returns the Symbol with the given name, if present.
func (t *Table) Lookup(name string) (*Symbol, bool) {
	s, ok := t.symbols[name]
	return s, ok
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
		t.symbols[name] = sym
	}
	return nil
}

// ReadCStringMem reads a null-terminated C string from memory at base+offset.
func ReadCStringMem(base, offset uintptr) string {
	ptr := base + offset
	var buf []byte
	for {
		b := *(*byte)(unsafe.Pointer(ptr))
		if b == 0 {
			break
		}
		buf = append(buf, b)
		ptr++
	}
	return string(buf)
}
