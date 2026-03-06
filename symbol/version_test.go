package symbol

import (
	"debug/elf"
	"testing"
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
