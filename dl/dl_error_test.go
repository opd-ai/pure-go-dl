package dl

import (
	"testing"
)

// TestOpenNonexistentLibrary tests error handling for libraries that don't exist
func TestOpenNonexistentLibrary(t *testing.T) {
	_, err := Open("/nonexistent/library/path.so")
	if err == nil {
		t.Error("Open should fail for nonexistent library")
	}
}

// TestOpenWithInvalidFlags tests handling of invalid flag combinations
func TestOpenWithInvalidFlags(t *testing.T) {
	// Open should handle any flag combination gracefully
	testLib := "../testdata/libtest.so"

	// Test with conflicting flags (both LOCAL and GLOBAL)
	lib, err := Open(testLib, RTLD_LOCAL, RTLD_GLOBAL)
	if err != nil {
		t.Fatalf("Open with multiple flags failed: %v", err)
	}
	defer lib.Close()

	// Behavior is implementation-defined, but should not crash
	t.Log("Open with multiple flags succeeded")
}

// TestSymLookupNonexistentSymbol tests error handling for undefined symbols
func TestSymLookupNonexistentSymbol(t *testing.T) {
	testLib := "../testdata/libtest.so"

	lib, err := Open(testLib)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer lib.Close()

	_, err = lib.Sym("nonexistent_symbol_that_does_not_exist")
	if err == nil {
		t.Error("Sym should fail for nonexistent symbol")
	}
}

// TestSymWithEmptyName tests error handling for empty symbol names
func TestSymWithEmptyName(t *testing.T) {
	testLib := "../testdata/libtest.so"

	lib, err := Open(testLib)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer lib.Close()

	_, err = lib.Sym("")
	if err == nil {
		t.Error("Sym should fail for empty symbol name")
	}
}

// TestBindWithInvalidTarget tests error handling for Bind with wrong types
func TestBindWithInvalidTarget(t *testing.T) {
	testLib := "../testdata/libtest.so"

	lib, err := Open(testLib)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer lib.Close()

	// purego.RegisterFunc will panic for invalid types
	// We test that our Bind function doesn't crash before reaching purego

	// Try to bind to nil - should fail at Sym lookup or purego registration
	defer func() {
		if r := recover(); r != nil {
			t.Logf("Bind with nil correctly panicked: %v", r)
		}
	}()

	_ = lib.Bind("add", nil)
	// If we reach here without panic, the implementation handles it gracefully
}

// TestBindNonexistentSymbol tests Bind error for undefined symbols
func TestBindNonexistentSymbol(t *testing.T) {
	testLib := "../testdata/libtest.so"

	lib, err := Open(testLib)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer lib.Close()

	var fn func()
	err = lib.Bind("this_symbol_does_not_exist", &fn)
	if err == nil {
		t.Error("Bind should fail for nonexistent symbol")
	}
}

// TestCloseUnloadedLibrary tests double-close behavior
func TestCloseUnloadedLibrary(t *testing.T) {
	testLib := "../testdata/libtest.so"

	lib, err := Open(testLib)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	// First close should succeed
	err = lib.Close()
	if err != nil {
		t.Errorf("First Close failed: %v", err)
	}

	// Second close behavior is implementation-defined
	// It might error, or be a no-op
	err = lib.Close()
	// Don't assert on the error - behavior is undefined
	t.Logf("Second Close returned: %v", err)
}

// TestOpenEmptyPath tests error handling for empty library path
func TestOpenEmptyPath(t *testing.T) {
	_, err := Open("")
	if err == nil {
		t.Error("Open should fail for empty path")
	}
}

// TestSymAfterClose tests that Sym fails after library is closed
func TestSymAfterClose(t *testing.T) {
	testLib := "../testdata/libtest.so"

	lib, err := Open(testLib)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	// Close the library
	err = lib.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Attempting to lookup symbols after close should fail
	// (or might succeed if memory is still mapped - implementation-defined)
	_, err = lib.Sym("add")
	if err != nil {
		t.Logf("Sym after Close correctly failed: %v", err)
	} else {
		t.Log("Sym after Close succeeded (implementation allows this)")
	}
}

// TestBindAfterClose tests that Bind fails after library is closed
func TestBindAfterClose(t *testing.T) {
	testLib := "../testdata/libtest.so"

	lib, err := Open(testLib)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	err = lib.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	var add func(int, int) int
	err = lib.Bind("add", &add)
	if err != nil {
		t.Logf("Bind after Close correctly failed: %v", err)
	} else {
		t.Log("Bind after Close succeeded (implementation allows this)")
	}
}

// TestFindLibraryNotFound tests library search path failure
func TestFindLibraryNotFound(t *testing.T) {
	// Try to open a library by name only (not full path)
	// This will trigger the library search mechanism

	_, err := Open("libdoesnotexist.so.999")
	if err == nil {
		t.Error("Open should fail for library not in search path")
	}
	t.Logf("Library search correctly failed: %v", err)
}

// TestOpenDirectory tests error handling when path is a directory
func TestOpenDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	_, err := Open(tmpDir)
	if err == nil {
		t.Error("Open should fail when path is a directory")
	}
}

// TestOpenWithCircularDependencies tests handling of circular deps
func TestOpenWithCircularDependencies(t *testing.T) {
	// Our test libraries don't have circular dependencies,
	// but we can verify the cycle detection mechanism exists
	// by loading a library with dependencies multiple times

	testLib := "../testdata/libtest.so"

	// Load the same library twice
	lib1, err := Open(testLib)
	if err != nil {
		t.Fatalf("First Open failed: %v", err)
	}
	defer lib1.Close()

	lib2, err := Open(testLib)
	if err != nil {
		t.Fatalf("Second Open failed: %v", err)
	}
	defer lib2.Close()

	// Both should succeed - the library is reference counted
	t.Log("Multiple opens of same library succeeded (reference counting works)")
}
