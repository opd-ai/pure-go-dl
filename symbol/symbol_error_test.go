package symbol

import (
	"testing"
)

// TestLoadFromDynamicWithZeroSymtab tests error handling when symtabAddr is zero
func TestLoadFromDynamicWithZeroSymtab(t *testing.T) {
	table := NewTable(0x1000)

	err := table.LoadFromDynamic(0, 0x2000, 100, 100)
	if err == nil {
		t.Error("LoadFromDynamic should fail with zero symtabAddr")
	}
}

// TestLookupInEmptyTable tests symbol lookup in an empty table
func TestLookupInEmptyTable(t *testing.T) {
	table := NewTable(0x1000)

	sym, found := table.Lookup("any_symbol")
	if found {
		t.Error("Lookup should not find symbol in empty table")
	}
	if sym != nil {
		t.Error("Lookup should return nil symbol when not found")
	}
}

// TestLookupNonexistentSymbol tests lookup of a symbol that doesn't exist
func TestLookupNonexistentSymbol(t *testing.T) {
	table := NewTable(0x1000)

	// Add a symbol
	table.AddSymbol("exists", &Symbol{
		Name:  "exists",
		Value: 0x2000,
	})

	// Look up a different symbol
	sym, found := table.Lookup("does_not_exist")
	if found {
		t.Error("Lookup should not find nonexistent symbol")
	}
	if sym != nil {
		t.Error("Lookup should return nil for nonexistent symbol")
	}
}

// TestLookupVersionWithoutVersion tests LookupVersion when table has no version info
func TestLookupVersionWithoutVersion(t *testing.T) {
	table := NewTable(0x1000)

	// Add an unversioned symbol
	table.AddSymbol("test", &Symbol{
		Name:    "test",
		Value:   0x2000,
		VerName: "",
	})

	// Look up with empty version string
	sym, found := table.LookupVersion("test", "")
	if !found {
		t.Error("LookupVersion with empty version should find unversioned symbol")
	}
	if sym == nil {
		t.Fatal("LookupVersion returned nil symbol")
	}
	if sym.Name != "test" {
		t.Errorf("Got wrong symbol: %s", sym.Name)
	}
}

// TestLookupVersionMismatch tests LookupVersion when versions don't match
func TestLookupVersionMismatch(t *testing.T) {
	table := NewTable(0x1000)

	// Add a versioned symbol
	table.AddSymbol("test", &Symbol{
		Name:    "test",
		Value:   0x2000,
		VerName: "GLIBC_2.2.5",
	})

	// Look up with different version
	sym, found := table.LookupVersion("test", "GLIBC_2.3.0")
	if found {
		t.Error("LookupVersion should not find symbol with mismatched version")
	}
	if sym != nil {
		t.Error("LookupVersion should return nil for version mismatch")
	}
}

// TestForEachEmpty tests ForEach on empty table
func TestForEachEmpty(t *testing.T) {
	table := NewTable(0x1000)

	called := false
	table.ForEach(func(name string, s *Symbol) {
		called = true
	})

	if called {
		t.Error("ForEach should not call function for empty table")
	}
}

// TestForEachMultipleSymbols tests ForEach iterates all symbols
func TestForEachMultipleSymbols(t *testing.T) {
	table := NewTable(0x1000)

	table.AddSymbol("sym1", &Symbol{Name: "sym1", Value: 0x1000})
	table.AddSymbol("sym2", &Symbol{Name: "sym2", Value: 0x2000})
	table.AddSymbol("sym3", &Symbol{Name: "sym3", Value: 0x3000})

	seen := make(map[string]bool)
	table.ForEach(func(name string, s *Symbol) {
		seen[name] = true
	})

	if len(seen) != 3 {
		t.Errorf("ForEach should have seen 3 symbols, got %d", len(seen))
	}
	for _, name := range []string{"sym1", "sym2", "sym3"} {
		if !seen[name] {
			t.Errorf("ForEach did not see symbol %s", name)
		}
	}
}
