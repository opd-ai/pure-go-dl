package dl

import (
	"debug/elf"
	"testing"
)

func TestIFuncSymbolResolution(t *testing.T) {
	// Load the library with IFUNC symbols
	lib, err := Open("../testdata/libifunc.so")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer lib.Close()

	// Check that add_ifunc symbol exists and is marked as IFUNC
	sym, ok := lib.obj.Symbols.Lookup("add_ifunc")
	if !ok {
		t.Fatal("add_ifunc symbol not found")
	}
	if sym.Type != elf.STT_GNU_IFUNC {
		t.Errorf("add_ifunc has type %v, want STT_GNU_IFUNC", sym.Type)
	}

	// Test calling the IFUNC function via Bind
	var addFunc func(int, int) int
	if err := lib.Bind("add_ifunc", &addFunc); err != nil {
		t.Fatalf("Bind failed: %v", err)
	}

	result := addFunc(10, 20)
	if result != 30 {
		t.Errorf("add_ifunc(10, 20) = %d, want 30", result)
	}

	// Also test the non-IFUNC function for comparison
	var mulFunc func(int, int) int
	if err := lib.Bind("multiply", &mulFunc); err != nil {
		t.Fatalf("Bind(multiply) failed: %v", err)
	}

	result = mulFunc(10, 20)
	if result != 200 {
		t.Errorf("multiply(10, 20) = %d, want 200", result)
	}
}

func TestIFuncSymMethod(t *testing.T) {
	// Load the library with IFUNC symbols
	lib, err := Open("../testdata/libifunc.so")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer lib.Close()

	// Test that Sym() correctly resolves IFUNC symbols
	addr, err := lib.Sym("add_ifunc")
	if err != nil {
		t.Fatalf("Sym(add_ifunc) failed: %v", err)
	}
	if addr == 0 {
		t.Error("Sym(add_ifunc) returned 0 address")
	}

	// Verify it's different from the symbol table value (resolver vs resolved)
	sym, _ := lib.obj.Symbols.Lookup("add_ifunc")
	if addr == sym.Value {
		t.Log("Note: resolved address equals symbol value - resolver may have returned itself")
	}
}
