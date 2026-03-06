package symbol

import (
	"debug/elf"
	"testing"
	"unsafe"
)

// TestNewTable verifies basic table creation and operations.
func TestNewTable(t *testing.T) {
	base := uintptr(0x400000)
	table := NewTable(base)

	if table == nil {
		t.Fatal("NewTable returned nil")
	}
	if table.base != base {
		t.Errorf("Expected base %#x, got %#x", base, table.base)
	}
}

// TestTable_AddSymbol_And_Lookup tests basic symbol insertion and retrieval.
func TestTable_AddSymbol_And_Lookup(t *testing.T) {
	table := NewTable(0x1000)

	sym := &Symbol{
		Name:  "test_func",
		Value: 0x1234,
		Size:  100,
		Bind:  elf.STB_GLOBAL,
		Type:  elf.STT_FUNC,
	}

	table.AddSymbol("test_func", sym)

	// Test successful lookup
	found, ok := table.Lookup("test_func")
	if !ok {
		t.Fatal("Lookup failed for existing symbol")
	}
	if found.Name != "test_func" || found.Value != 0x1234 {
		t.Errorf("Symbol data mismatch: got %+v", found)
	}

	// Test failed lookup
	_, ok = table.Lookup("nonexistent")
	if ok {
		t.Error("Lookup succeeded for nonexistent symbol")
	}
}

// TestTable_SetVersionTable verifies version table assignment.
func TestTable_SetVersionTable(t *testing.T) {
	table := NewTable(0x1000)
	if table.versions != nil {
		t.Error("New table should have nil version table")
	}

	vt := NewVersionTable()
	table.SetVersionTable(vt)

	if table.versions != vt {
		t.Error("SetVersionTable did not assign version table correctly")
	}
}

// TestTable_ForEach verifies iteration over all symbols.
func TestTable_ForEach(t *testing.T) {
	table := NewTable(0x1000)

	symbols := []*Symbol{
		{Name: "alpha", Value: 0x1000},
		{Name: "beta", Value: 0x2000},
		{Name: "gamma", Value: 0x3000},
	}

	for _, sym := range symbols {
		table.AddSymbol(sym.Name, sym)
	}

	found := make(map[string]bool)
	table.ForEach(func(name string, s *Symbol) {
		found[name] = true
		// Verify the symbol matches what we inserted
		for _, expected := range symbols {
			if expected.Name == name && expected.Value != s.Value {
				t.Errorf("Symbol %s has wrong value: got %#x, want %#x",
					name, s.Value, expected.Value)
			}
		}
	})

	if len(found) != len(symbols) {
		t.Errorf("ForEach visited %d symbols, expected %d", len(found), len(symbols))
	}
	for _, sym := range symbols {
		if !found[sym.Name] {
			t.Errorf("ForEach did not visit symbol %s", sym.Name)
		}
	}
}

// TestReadCStringMem tests reading null-terminated strings from memory.
func TestReadCStringMem(t *testing.T) {
	// Create a memory buffer with test strings
	testData := []byte("hello\x00world\x00test\x00")
	baseAddr := uintptr(unsafe.Pointer(&testData[0]))

	tests := []struct {
		offset uintptr
		want   string
	}{
		{0, "hello"},
		{6, "world"},
		{12, "test"},
	}

	for _, tt := range tests {
		got := ReadCStringMem(baseAddr, tt.offset)
		if got != tt.want {
			t.Errorf("ReadCStringMem(base, %d) = %q, want %q", tt.offset, got, tt.want)
		}
	}
}

// TestReadCStringMem_Empty tests reading an empty string.
func TestReadCStringMem_Empty(t *testing.T) {
	testData := []byte("\x00remaining")
	baseAddr := uintptr(unsafe.Pointer(&testData[0]))

	got := ReadCStringMem(baseAddr, 0)
	if got != "" {
		t.Errorf("ReadCStringMem for empty string = %q, want empty string", got)
	}
}

// TestLoadFromDynamic_ErrorCases tests error handling in LoadFromDynamic.
func TestLoadFromDynamic_ErrorCases(t *testing.T) {
	table := NewTable(0x1000)

	// Test with zero symtab address
	err := table.LoadFromDynamic(0, 0x2000, 100)
	if err == nil {
		t.Error("LoadFromDynamic with zero symtabAddr should return error")
	}
}

// TestLoadFromDynamic_BasicSymbols tests loading symbols from a mock dynamic section.
func TestLoadFromDynamic_BasicSymbols(t *testing.T) {
	// Skip this test when running with -race as it uses synthetic memory
	// that doesn't pass checkptr validation. Integration tests with real
	// ELF files (in dl/ and loader/ packages) provide coverage with -race.
	if RaceEnabled {
		t.Skip("Skipping synthetic memory test with -race (checkptr incompatible)")
	}

	// Create mock symbol table and string table in memory.
	// Layout:
	//   - Symbol 0: NULL entry (name=0, value=0)
	//   - Symbol 1: "testfunc" (global function)
	//   - Symbol 2: "testvar" (global object)

	// String table: "\x00testfunc\x00testvar\x00"
	strtab := []byte("\x00testfunc\x00testvar\x00")
	strtabAddr := uintptr(unsafe.Pointer(&strtab[0]))

	// Symbol table - allocate as actual structs for checkptr compatibility
	const numSymbols = 100
	symbols := make([]Elf64Sym, numSymbols)

	// Symbol 0: NULL
	symbols[0] = Elf64Sym{Name: 0, Info: 0, Other: 0, Shndx: 0, Value: 0, Size: 0}

	// Symbol 1: testfunc (GLOBAL, FUNC, defined in section 1)
	symbols[1] = Elf64Sym{
		Name:  1, // offset into strtab
		Info:  byte(uint8(elf.STB_GLOBAL)<<4 | uint8(elf.STT_FUNC)&0xf),
		Other: 0,
		Shndx: 1,
		Value: 0x2000,
		Size:  64,
	}

	// Symbol 2: testvar (GLOBAL, OBJECT, defined in section 2)
	symbols[2] = Elf64Sym{
		Name:  10, // offset into strtab
		Info:  byte(uint8(elf.STB_GLOBAL)<<4 | uint8(elf.STT_OBJECT)&0xf),
		Other: 0,
		Shndx: 2,
		Value: 0x3000,
		Size:  8,
	}

	symtabAddr := uintptr(unsafe.Pointer(&symbols[0]))
	symtabSize := uint64(3 * symEntSize)

	table := NewTable(0x10000)
	err := table.LoadFromDynamic(symtabAddr, strtabAddr, symtabSize)
	if err != nil {
		t.Fatalf("LoadFromDynamic failed: %v", err)
	}

	// Verify testfunc was loaded
	sym, ok := table.Lookup("testfunc")
	if !ok {
		t.Fatal("testfunc not found in table")
	}
	if sym.Name != "testfunc" {
		t.Errorf("Symbol name = %q, want testfunc", sym.Name)
	}
	// Value should be base + st_value
	expectedValue := uintptr(0x10000 + 0x2000)
	if sym.Value != expectedValue {
		t.Errorf("testfunc value = %#x, want %#x", sym.Value, expectedValue)
	}
	if sym.Bind != elf.STB_GLOBAL {
		t.Errorf("testfunc bind = %v, want STB_GLOBAL", sym.Bind)
	}
	if sym.Type != elf.STT_FUNC {
		t.Errorf("testfunc type = %v, want STT_FUNC", sym.Type)
	}

	// Verify testvar was loaded
	sym, ok = table.Lookup("testvar")
	if !ok {
		t.Fatal("testvar not found in table")
	}
	expectedValue = uintptr(0x10000 + 0x3000)
	if sym.Value != expectedValue {
		t.Errorf("testvar value = %#x, want %#x", sym.Value, expectedValue)
	}
}

// TestLoadFromDynamic_SkipsUndefined tests that undefined symbols are skipped.
func TestLoadFromDynamic_SkipsUndefined(t *testing.T) {
	if RaceEnabled {
		t.Skip("Skipping synthetic memory test with -race (checkptr incompatible)")
	}

	strtab := []byte("\x00undefined_func\x00")
	strtabAddr := uintptr(unsafe.Pointer(&strtab[0]))

	// Create a symbol with SHN_UNDEF section index
	symbols := make([]Elf64Sym, 1)
	symbols[0] = Elf64Sym{
		Name:  1,
		Info:  byte(elf.STB_GLOBAL << 4),
		Other: 0,
		Shndx: uint16(elf.SHN_UNDEF), // undefined symbol
		Value: 0,
		Size:  0,
	}
	symtabAddr := uintptr(unsafe.Pointer(&symbols[0]))
	symtabSize := uint64(1 * symEntSize)

	table := NewTable(0x1000)
	err := table.LoadFromDynamic(symtabAddr, strtabAddr, symtabSize)
	if err != nil {
		t.Fatalf("LoadFromDynamic failed: %v", err)
	}

	// Undefined symbol should not be in the table
	_, ok := table.Lookup("undefined_func")
	if ok {
		t.Error("Undefined symbol should not be added to table")
	}
}

// TestLoadFromDynamic_SkipsLocal tests that local symbols are skipped.
func TestLoadFromDynamic_SkipsLocal(t *testing.T) {
	if RaceEnabled {
		t.Skip("Skipping synthetic memory test with -race (checkptr incompatible)")
	}

	strtab := []byte("\x00local_func\x00")
	strtabAddr := uintptr(unsafe.Pointer(&strtab[0]))

	symbols := make([]Elf64Sym, 10)
	symbols[0] = Elf64Sym{
		Name:  1,
		Info:  byte(uint8(elf.STB_LOCAL)<<4 | uint8(elf.STT_FUNC)&0xf),
		Other: 0,
		Shndx: 1,
		Value: 0x1000,
		Size:  32,
	}
	symtabAddr := uintptr(unsafe.Pointer(&symbols[0]))
	symtabSize := uint64(1 * symEntSize)

	table := NewTable(0x1000)
	err := table.LoadFromDynamic(symtabAddr, strtabAddr, symtabSize)
	if err != nil {
		t.Fatalf("LoadFromDynamic failed: %v", err)
	}

	// Local symbol should not be added
	_, ok := table.Lookup("local_func")
	if ok {
		t.Error("Local symbol should not be added to table")
	}
}

// TestLoadFromDynamic_WeakSymbols tests that weak symbols are loaded.
func TestLoadFromDynamic_WeakSymbols(t *testing.T) {
	if RaceEnabled {
		t.Skip("Skipping synthetic memory test with -race (checkptr incompatible)")
	}

	strtab := []byte("\x00weak_func\x00")
	strtabAddr := uintptr(unsafe.Pointer(&strtab[0]))

	symbols := make([]Elf64Sym, 10)
	symbols[0] = Elf64Sym{
		Name:  1,
		Info:  byte(uint8(elf.STB_WEAK)<<4 | uint8(elf.STT_FUNC)&0xf),
		Other: 0,
		Shndx: 1,
		Value: 0x1000,
		Size:  32,
	}
	symtabAddr := uintptr(unsafe.Pointer(&symbols[0]))
	symtabSize := uint64(1 * symEntSize)

	table := NewTable(0x1000)
	err := table.LoadFromDynamic(symtabAddr, strtabAddr, symtabSize)
	if err != nil {
		t.Fatalf("LoadFromDynamic failed: %v", err)
	}

	// Weak symbol should be loaded
	sym, ok := table.Lookup("weak_func")
	if !ok {
		t.Fatal("Weak symbol should be added to table")
	}
	if sym.Bind != elf.STB_WEAK {
		t.Errorf("Symbol bind = %v, want STB_WEAK", sym.Bind)
	}
}

// TestLoadFromDynamic_WithVersionInfo tests loading symbols with version information.
func TestLoadFromDynamic_WithVersionInfo(t *testing.T) {
	if RaceEnabled {
		t.Skip("Skipping synthetic memory test with -race (checkptr incompatible)")
	}

	strtab := []byte("\x00versioned_func\x00")
	strtabAddr := uintptr(unsafe.Pointer(&strtab[0]))

	symbols := make([]Elf64Sym, 10)
	// Symbol 0: NULL
	symbols[0] = Elf64Sym{Name: 0, Info: 0, Other: 0, Shndx: 0, Value: 0, Size: 0}
	// Symbol 1: versioned_func
	symbols[1] = Elf64Sym{
		Name:  1,
		Info:  byte(uint8(elf.STB_GLOBAL)<<4 | uint8(elf.STT_FUNC)&0xf),
		Other: 0,
		Shndx: 1,
		Value: 0x2000,
		Size:  64,
	}
	symtabAddr := uintptr(unsafe.Pointer(&symbols[0]))
	symtabSize := uint64(2 * symEntSize)

	table := NewTable(0x1000)

	// Create and attach version table
	vt := NewVersionTable()
	vt.SymbolVersions = []uint16{0, 2} // Symbol 0 has version 0, symbol 1 has version 2
	vt.Requirements[2] = &VersionRequirement{Name: "GLIBC_2.2.5", Index: 2}
	table.SetVersionTable(vt)

	err := table.LoadFromDynamic(symtabAddr, strtabAddr, symtabSize)
	if err != nil {
		t.Fatalf("LoadFromDynamic failed: %v", err)
	}

	sym, ok := table.Lookup("versioned_func")
	if !ok {
		t.Fatal("versioned_func not found")
	}
	if sym.VerIdx != 2 {
		t.Errorf("Symbol version index = %d, want 2", sym.VerIdx)
	}
	if sym.VerName != "GLIBC_2.2.5" {
		t.Errorf("Symbol version name = %q, want GLIBC_2.2.5", sym.VerName)
	}
}

// Benchmarks for performance-critical symbol operations

// BenchmarkTableLookup measures symbol lookup performance.
func BenchmarkTableLookup(b *testing.B) {
	table := NewTable(0x1000)

	// Add 1000 symbols to make it realistic
	for i := 0; i < 1000; i++ {
		name := string(rune('a'+(i%26))) + string(rune('0'+(i/26)))
		sym := &Symbol{
			Name:  name,
			Value: uintptr(0x1000 + i*16),
			Size:  16,
			Bind:  elf.STB_GLOBAL,
			Type:  elf.STT_FUNC,
		}
		table.AddSymbol(name, sym)
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = table.Lookup("a0")
	}
}

// BenchmarkTableAddSymbol measures symbol insertion performance.
func BenchmarkTableAddSymbol(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		table := NewTable(0x1000)
		sym := &Symbol{
			Name:  "test_func",
			Value: 0x1234,
			Size:  100,
			Bind:  elf.STB_GLOBAL,
			Type:  elf.STT_FUNC,
		}
		table.AddSymbol("test_func", sym)
	}
}

// BenchmarkReadCStringMem measures string reading performance.
func BenchmarkReadCStringMem(b *testing.B) {
	testData := []byte("benchmark_test_string\x00")
	baseAddr := uintptr(unsafe.Pointer(&testData[0]))

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = ReadCStringMem(baseAddr, 0)
	}
}
