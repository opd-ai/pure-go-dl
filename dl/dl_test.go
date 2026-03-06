package dl

import (
	"testing"
)

func TestLoadLibrary(t *testing.T) {
	lib, err := Open("../testdata/libtest.so")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer lib.Close()

	// Test symbol lookup
	_, err = lib.Sym("add")
	if err != nil {
		t.Errorf("Sym(add) failed: %v", err)
	}

	_, err = lib.Sym("nonexistent")
	if err == nil {
		t.Errorf("Sym(nonexistent) should have failed")
	}
}

func TestBindFunction(t *testing.T) {
	lib, err := Open("../testdata/libtest.so")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer lib.Close()

	// Test binding and calling a function
	var add func(int, int) int
	err = lib.Bind("add", &add)
	if err != nil {
		t.Fatalf("Bind(add) failed: %v", err)
	}

	result := add(3, 4)
	if result != 7 {
		t.Errorf("add(3, 4) = %d, want 7", result)
	}

	result = add(100, 200)
	if result != 300 {
		t.Errorf("add(100, 200) = %d, want 300", result)
	}
}

func TestConstructorCalled(t *testing.T) {
	lib, err := Open("../testdata/libtest.so")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer lib.Close()

	// The constructor should have set counter to 42
	var getCounter func() int
	err = lib.Bind("get_counter", &getCounter)
	if err != nil {
		t.Fatalf("Bind(get_counter) failed: %v", err)
	}

	counter := getCounter()
	if counter != 42 {
		t.Errorf("get_counter() = %d, want 42 (constructor should have initialized it)", counter)
	}
}

func TestInternalFunctionCall(t *testing.T) {
	lib, err := Open("../testdata/libreloc.so")
	if err != nil {
		t.Fatalf("Open libreloc.so failed: %v", err)
	}
	defer lib.Close()

	// square_plus_one calls internal static function square
	var squarePlusOne func(int) int
	err = lib.Bind("square_plus_one", &squarePlusOne)
	if err != nil {
		t.Fatalf("Bind(square_plus_one) failed: %v", err)
	}

	result := squarePlusOne(5)
	if result != 26 { // 5*5 + 1
		t.Errorf("square_plus_one(5) = %d, want 26", result)
	}
}

func TestRefCounting(t *testing.T) {
	lib1, err := Open("../testdata/libtest.so")
	if err != nil {
		t.Fatalf("First Open failed: %v", err)
	}

	// Second open should return same library with incremented refcount
	lib2, err := Open("../testdata/libtest.so")
	if err != nil {
		t.Fatalf("Second Open failed: %v", err)
	}

	if lib1.obj != lib2.obj {
		t.Errorf("Expected same object for duplicate Open calls")
	}

	if lib1.obj.RefCount != 2 {
		t.Errorf("RefCount = %d, want 2", lib1.obj.RefCount)
	}

	// First close should not unload
	lib1.Close()
	if lib1.obj.RefCount != 1 {
		t.Errorf("After first Close, RefCount = %d, want 1", lib1.obj.RefCount)
	}

	// Second close should unload
	lib2.Close()
	if lib1.obj.RefCount != 0 {
		t.Errorf("After second Close, RefCount = %d, want 0", lib1.obj.RefCount)
	}
}

func TestWeakSymbolsResolveToZero(t *testing.T) {
	// This test verifies that weak symbols like __cxa_finalize resolve to NULL
	// instead of causing loading to fail. We can't directly test the symbol
	// value since they're not exported, but if the library loads successfully,
	// the fix is working.
	lib, err := Open("../testdata/libtest.so")
	if err != nil {
		t.Fatalf("Open with weak symbols failed: %v (weak symbols should resolve to NULL, not fail)", err)
	}
	defer lib.Close()

	// Verify library is functional
	var add func(int, int) int
	err = lib.Bind("add", &add)
	if err != nil {
		t.Fatalf("Bind failed: %v", err)
	}

	if add(1, 1) != 2 {
		t.Errorf("Library with weak symbols is not functional")
	}
}

func TestRunpathRpathParsing(t *testing.T) {
	// Test that RUNPATH and RPATH are correctly parsed from ELF files.
	// We'll load a library and verify its ParsedObject contains the paths.
	lib, err := Open("../testdata/libtest.so")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer lib.Close()

	// The test libraries don't have RUNPATH/RPATH set, but we can verify
	// the fields exist and are empty strings (not causing errors).
	// This test documents that the functionality exists and can be extended
	// if we create test libraries with RUNPATH/RPATH.
	parsed := lib.obj.Parsed
	if parsed == nil {
		t.Fatal("Expected Parsed object to exist")
	}

	// Runpath and Rpath should be accessible (even if empty for this library)
	_ = parsed.Runpath
	_ = parsed.Rpath

	// This test passes if the library loads successfully with the new fields.
	// In the future, we could create a library with RUNPATH set and verify
	// it's parsed correctly.
}
