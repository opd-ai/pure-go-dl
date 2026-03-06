package symbol

import (
	"debug/elf"
	"testing"
	"unsafe"
)

func TestVersionTable_ParseVersionTables(t *testing.T) {
	// Test with empty/nil addresses.
	vt := NewVersionTable()
	err := vt.ParseVersionTables(0, 0, 0, 0, 0, 0, 0)
	if err != nil {
		t.Errorf("ParseVersionTables with nil addresses should not error: %v", err)
	}

	if len(vt.Requirements) != 0 || len(vt.Definitions) != 0 {
		t.Errorf("Empty version table should have no requirements or definitions")
	}
}

func TestVersionTable_ParseVerneed(t *testing.T) {
	// Create a simple Verneed chain in memory.
	// String table for version names.
	strtab := []byte("\x00GLIBC_2.2.5\x00GLIBC_2.17\x00")
	strtabAddr := uintptr(unsafe.Pointer(&strtab[0]))

	// Build a Verneed structure:
	// Verneed header (16 bytes):
	//   uint16 vn_version = 1
	//   uint16 vn_cnt = 2 (two Vernaux entries)
	//   uint32 vn_file = 0 (library name offset, not used)
	//   uint32 vn_aux = 16 (offset to first Vernaux)
	//   uint32 vn_next = 0 (no next Verneed)
	// Vernaux 1 (16 bytes):
	//   uint32 vna_hash = 0 (not used)
	//   uint16 vna_flags = 0
	//   uint16 vna_other = 2 (version index)
	//   uint32 vna_name = 1 (offset to "GLIBC_2.2.5")
	//   uint32 vna_next = 16 (offset to next Vernaux)
	// Vernaux 2 (16 bytes):
	//   uint32 vna_hash = 0
	//   uint16 vna_flags = 0
	//   uint16 vna_other = 3 (version index)
	//   uint32 vna_name = 13 (offset to "GLIBC_2.17")
	//   uint32 vna_next = 0 (end of chain)

	verneedData := []byte{
		// Verneed header (16 bytes)
		0x01, 0x00, // vn_version = 1
		0x02, 0x00, // vn_cnt = 2
		0x00, 0x00, 0x00, 0x00, // vn_file = 0
		0x10, 0x00, 0x00, 0x00, // vn_aux = 16
		0x00, 0x00, 0x00, 0x00, // vn_next = 0
		// Vernaux 1 (16 bytes) - starts at offset 16
		0x00, 0x00, 0x00, 0x00, // vna_hash
		0x00, 0x00, // vna_flags
		0x02, 0x00, // vna_other = 2
		0x01, 0x00, 0x00, 0x00, // vna_name = 1
		0x10, 0x00, 0x00, 0x00, // vna_next = 16
		// Vernaux 2 (16 bytes) - starts at offset 32
		0x00, 0x00, 0x00, 0x00, // vna_hash
		0x00, 0x00, // vna_flags
		0x03, 0x00, // vna_other = 3
		0x0d, 0x00, 0x00, 0x00, // vna_name = 13
		0x00, 0x00, 0x00, 0x00, // vna_next = 0
	}
	verneedAddr := uintptr(unsafe.Pointer(&verneedData[0]))

	vt := NewVersionTable()
	err := vt.ParseVersionTables(0, 0, verneedAddr, 1, 0, 0, strtabAddr)
	if err != nil {
		t.Fatalf("ParseVersionTables failed: %v", err)
	}

	// Verify requirements were parsed
	if len(vt.Requirements) != 2 {
		t.Errorf("Expected 2 requirements, got %d", len(vt.Requirements))
	}

	req, ok := vt.Requirements[2]
	if !ok {
		t.Fatal("Requirement with index 2 not found")
	}
	if req.Name != "GLIBC_2.2.5" {
		t.Errorf("Requirement 2 name = %q, want GLIBC_2.2.5", req.Name)
	}

	req, ok = vt.Requirements[3]
	if !ok {
		t.Fatal("Requirement with index 3 not found")
	}
	if req.Name != "GLIBC_2.17" {
		t.Errorf("Requirement 3 name = %q, want GLIBC_2.17", req.Name)
	}
}

func TestVersionTable_ParseVerdef(t *testing.T) {
	// Create a simple Verdef chain in memory.
	strtab := []byte("\x00MYLIB_1.0\x00MYLIB_2.0\x00")
	strtabAddr := uintptr(unsafe.Pointer(&strtab[0]))

	// Build Verdef structures:
	// Verdef 1 (20 bytes):
	//   uint16 vd_version = 1
	//   uint16 vd_flags = 0
	//   uint16 vd_ndx = 1 (version index)
	//   uint16 vd_cnt = 1 (one Verdaux)
	//   uint32 vd_hash = 0
	//   uint32 vd_aux = 20 (offset to Verdaux)
	//   uint32 vd_next = 28 (offset to next Verdef)
	// Verdaux 1 (8 bytes):
	//   uint32 vda_name = 1 (offset to "MYLIB_1.0")
	//   uint32 vda_next = 0
	// Verdef 2 (20 bytes):
	//   uint16 vd_version = 1
	//   uint16 vd_flags = 0
	//   uint16 vd_ndx = 2
	//   uint16 vd_cnt = 1
	//   uint32 vd_hash = 0
	//   uint32 vd_aux = 20
	//   uint32 vd_next = 0
	// Verdaux 2 (8 bytes):
	//   uint32 vda_name = 11 (offset to "MYLIB_2.0")
	//   uint32 vda_next = 0

	verdefData := []byte{
		// Verdef 1
		0x01, 0x00, // vd_version = 1
		0x00, 0x00, // vd_flags = 0
		0x01, 0x00, // vd_ndx = 1
		0x01, 0x00, // vd_cnt = 1
		0x00, 0x00, 0x00, 0x00, // vd_hash
		0x14, 0x00, 0x00, 0x00, // vd_aux = 20
		0x1c, 0x00, 0x00, 0x00, // vd_next = 28
		// Verdaux 1
		0x01, 0x00, 0x00, 0x00, // vda_name = 1
		0x00, 0x00, 0x00, 0x00, // vda_next = 0
		// Verdef 2
		0x01, 0x00, // vd_version = 1
		0x00, 0x00, // vd_flags = 0
		0x02, 0x00, // vd_ndx = 2
		0x01, 0x00, // vd_cnt = 1
		0x00, 0x00, 0x00, 0x00, // vd_hash
		0x14, 0x00, 0x00, 0x00, // vd_aux = 20
		0x00, 0x00, 0x00, 0x00, // vd_next = 0
		// Verdaux 2
		0x0b, 0x00, 0x00, 0x00, // vda_name = 11
		0x00, 0x00, 0x00, 0x00, // vda_next = 0
	}
	verdefAddr := uintptr(unsafe.Pointer(&verdefData[0]))

	vt := NewVersionTable()
	err := vt.ParseVersionTables(0, 0, 0, 0, verdefAddr, 2, strtabAddr)
	if err != nil {
		t.Fatalf("ParseVersionTables failed: %v", err)
	}

	// Verify definitions were parsed
	if len(vt.Definitions) != 2 {
		t.Errorf("Expected 2 definitions, got %d", len(vt.Definitions))
	}

	def, ok := vt.Definitions[1]
	if !ok {
		t.Fatal("Definition with index 1 not found")
	}
	if def.Name != "MYLIB_1.0" {
		t.Errorf("Definition 1 name = %q, want MYLIB_1.0", def.Name)
	}

	def, ok = vt.Definitions[2]
	if !ok {
		t.Fatal("Definition with index 2 not found")
	}
	if def.Name != "MYLIB_2.0" {
		t.Errorf("Definition 2 name = %q, want MYLIB_2.0", def.Name)
	}
}

func TestParseDynTags(t *testing.T) {
	// Create a mock dynamic section with version-related tags.
	// Each entry is 16 bytes (8 bytes tag, 8 bytes value).
	dynData := []byte{
		// DT_VERSYM = 0x6ffffff0
		0xf0, 0xff, 0xff, 0x6f, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x10, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // value = 0x1000
		// DT_VERNEED = 0x6ffffffe
		0xfe, 0xff, 0xff, 0x6f, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x20, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // value = 0x2000
		// DT_VERNEEDNUM = 0x6fffffff
		0xff, 0xff, 0xff, 0x6f, 0x00, 0x00, 0x00, 0x00,
		0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // value = 2
		// DT_NULL = 0
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	}

	tags := ParseDynTags(dynData)

	if val, ok := tags[elf.DT_VERSYM]; !ok || val != 0x1000 {
		t.Errorf("DT_VERSYM = %#x, want 0x1000", val)
	}
	if val, ok := tags[elf.DT_VERNEED]; !ok || val != 0x2000 {
		t.Errorf("DT_VERNEED = %#x, want 0x2000", val)
	}
	if val, ok := tags[elf.DT_VERNEEDNUM]; !ok || val != 2 {
		t.Errorf("DT_VERNEEDNUM = %#x, want 2", val)
	}
}

func TestParseDynTags_Empty(t *testing.T) {
	// Test with empty data.
	tags := ParseDynTags([]byte{})
	if len(tags) != 0 {
		t.Errorf("Expected empty tags map, got %d entries", len(tags))
	}
}

func TestParseDynTags_NullTerminated(t *testing.T) {
	// Test that parsing stops at DT_NULL.
	dynData := []byte{
		// Some tag
		0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0xff, 0xff, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		// DT_NULL
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		// More data that should be ignored
		0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0xaa, 0xaa, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	}

	tags := ParseDynTags(dynData)

	// Should only have one tag (before DT_NULL).
	if len(tags) != 1 {
		t.Errorf("Expected 1 tag, got %d", len(tags))
	}
	if _, ok := tags[elf.DynTag(2)]; ok {
		t.Error("Tag after DT_NULL should not be parsed")
	}
}

func TestVersionTable_GetSymbolVersion(t *testing.T) {
	vt := NewVersionTable()

	// Test with no symbol versions loaded.
	ver := vt.GetSymbolVersion(0)
	if ver != 0 {
		t.Errorf("Expected version 0 for empty table, got %d", ver)
	}

	// Simulate loading some symbol versions.
	vt.SymbolVersions = []uint16{0, 1, 2, 0x8003} // 0x8003 has hidden bit set

	if ver := vt.GetSymbolVersion(0); ver != 0 {
		t.Errorf("Symbol 0: expected version 0, got %d", ver)
	}
	if ver := vt.GetSymbolVersion(1); ver != 1 {
		t.Errorf("Symbol 1: expected version 1, got %d", ver)
	}
	if ver := vt.GetSymbolVersion(2); ver != 2 {
		t.Errorf("Symbol 2: expected version 2, got %d", ver)
	}
	// Hidden bit should be masked off.
	if ver := vt.GetSymbolVersion(3); ver != 3 {
		t.Errorf("Symbol 3: expected version 3 (hidden bit masked), got %d", ver)
	}
}

func TestVersionTable_GetVersionName(t *testing.T) {
	vt := NewVersionTable()

	// Add some test requirements and definitions.
	vt.Requirements[2] = &VersionRequirement{Name: "GLIBC_2.2.5", Index: 2}
	vt.Requirements[3] = &VersionRequirement{Name: "GLIBC_2.17", Index: 3}
	vt.Definitions[1] = &VersionDefinition{Name: "MYLIB_1.0", Index: 1}

	tests := []struct {
		verIdx uint16
		want   string
	}{
		{0, ""},
		{1, "MYLIB_1.0"},
		{2, "GLIBC_2.2.5"},
		{3, "GLIBC_2.17"},
		{99, ""},
	}

	for _, tt := range tests {
		got := vt.GetVersionName(tt.verIdx)
		if got != tt.want {
			t.Errorf("GetVersionName(%d) = %q, want %q", tt.verIdx, got, tt.want)
		}
	}
}

func TestParseVersionInfo(t *testing.T) {
	// Test with no version tags.
	dynEntries := map[elf.DynTag]uint64{}
	vt, err := ParseVersionInfo(dynEntries, 0, 0, 0)
	if err != nil {
		t.Errorf("ParseVersionInfo with empty tags should not error: %v", err)
	}
	if vt == nil {
		t.Fatal("Expected non-nil version table")
	}

	// Test with version tags present but addresses are 0 (no actual data).
	dynEntries[elf.DT_VERSYM] = 0x1000
	dynEntries[elf.DT_VERNEED] = 0x2000
	dynEntries[elf.DT_VERNEEDNUM] = 0
	dynEntries[elf.DT_VERDEF] = 0x3000
	dynEntries[elf.DT_VERDEFNUM] = 0

	vt, err = ParseVersionInfo(dynEntries, 0, 0, 0)
	if err != nil {
		t.Errorf("ParseVersionInfo should handle zero counts gracefully: %v", err)
	}
	if vt == nil {
		t.Fatal("Expected non-nil version table")
	}
}

func TestSymbol_LookupVersion(t *testing.T) {
	table := NewTable(0x1000)

	// Add a symbol with version.
	sym := &Symbol{
		Name:    "foo",
		Value:   0x1234,
		VerIdx:  2,
		VerName: "GLIBC_2.2.5",
	}
	table.AddSymbol("foo", sym)

	// Lookup without version should work.
	s, ok := table.Lookup("foo")
	if !ok || s.Name != "foo" {
		t.Errorf("Lookup(foo) failed")
	}

	// Lookup with matching version should work.
	s, ok = table.LookupVersion("foo", "GLIBC_2.2.5")
	if !ok || s.Name != "foo" {
		t.Errorf("LookupVersion(foo, GLIBC_2.2.5) failed")
	}

	// Lookup with non-matching version should fail.
	_, ok = table.LookupVersion("foo", "GLIBC_2.17")
	if ok {
		t.Errorf("LookupVersion(foo, GLIBC_2.17) should have failed")
	}

	// Lookup with empty version should work (default behavior).
	s, ok = table.LookupVersion("foo", "")
	if !ok || s.Name != "foo" {
		t.Errorf("LookupVersion(foo, '') failed")
	}
}
