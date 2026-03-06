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

func TestDoubleCloseFails(t *testing.T) {
	lib, err := Open("../testdata/libtest.so")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	// First close should succeed
	err = lib.Close()
	if err != nil {
		t.Fatalf("First Close failed: %v", err)
	}

	// Second close should fail with an error
	err = lib.Close()
	if err == nil {
		t.Errorf("Second Close should have failed but succeeded")
	}
	if err != nil && err.Error() != "dl: Close() called more than Open()" {
		t.Errorf("Expected 'dl: Close() called more than Open()', got %q", err.Error())
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

func TestRTLD_NOW_Flag(t *testing.T) {
	// Test that RTLD_NOW flag is accepted (compatibility with standard dlopen).
	// Since all loading is eager binding, RTLD_NOW should behave like RTLD_LOCAL.
	lib, err := Open("../testdata/libtest.so", RTLD_NOW)
	if err != nil {
		t.Fatalf("Open with RTLD_NOW failed: %v", err)
	}
	defer lib.Close()

	// Verify the library is functional
	var add func(int, int) int
	err = lib.Bind("add", &add)
	if err != nil {
		t.Fatalf("Bind failed: %v", err)
	}

	result := add(10, 20)
	if result != 30 {
		t.Errorf("add(10, 20) = %d, want 30", result)
	}

	// Verify it's not global (RTLD_NOW should act like RTLD_LOCAL)
	if lib.global {
		t.Errorf("Library loaded with RTLD_NOW should not be global")
	}
}

// TestTLSLibraryParsing tests that libraries with TLS segments can be loaded
// and that TLS metadata is correctly parsed.
func TestTLSLibraryParsing(t *testing.T) {
	// Test loading a library with Thread-Local Storage (TLS).
	lib, err := Open("../testdata/libtls.so")
	if err != nil {
		t.Fatalf("Open libtls.so failed: %v", err)
	}
	defer lib.Close()

	// Verify the library loaded and has TLS module registered
	if lib.obj.TLSModule == nil {
		t.Error("Expected TLS module to be registered")
	} else {
		t.Logf("TLS module ID: %d, Size: %d, Align: %d",
			lib.obj.TLSModule.ID,
			lib.obj.TLSModule.Size,
			lib.obj.TLSModule.Align)
	}

	// Verify we can look up TLS symbols
	_, err = lib.Sym("get_tls_counter")
	if err != nil {
		t.Errorf("Sym(get_tls_counter) failed: %v", err)
	}

	// Test calling TLS functions now that __tls_get_addr is implemented
	var getTLSCounter func() int32
	err = lib.Bind("get_tls_counter", &getTLSCounter)
	if err != nil {
		t.Fatalf("Bind(get_tls_counter) failed: %v", err)
	}

	// The initial value should be 42 (set in libtls.c)
	value := getTLSCounter()
	if value != 42 {
		t.Errorf("get_tls_counter() = %d, want 42", value)
	} else {
		t.Logf("get_tls_counter() = %d ✓", value)
	}

	// Test incrementing the counter
	var incrementTLSCounter func() int32
	err = lib.Bind("increment_tls_counter", &incrementTLSCounter)
	if err != nil {
		t.Fatalf("Bind(increment_tls_counter) failed: %v", err)
	}

	newValue := incrementTLSCounter()
	if newValue != 43 {
		t.Errorf("increment_tls_counter() = %d, want 43", newValue)
	} else {
		t.Logf("increment_tls_counter() = %d ✓", newValue)
	}

	// Verify the counter was actually incremented
	currentValue := getTLSCounter()
	if currentValue != 43 {
		t.Errorf("After increment, get_tls_counter() = %d, want 43", currentValue)
	}
}
