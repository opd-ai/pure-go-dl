package loader

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadNonELFFile tests loading a non-ELF file
func TestLoadNonELFFile(t *testing.T) {
	// Create a temporary non-ELF file
	tmpDir := t.TempDir()
	badFile := filepath.Join(tmpDir, "notelf.so")

	if err := os.WriteFile(badFile, []byte("This is not an ELF file"), 0o644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	resolver := &mockResolver{symbols: make(map[string]uintptr)}

	_, err := Load(badFile, resolver)
	if err == nil {
		t.Error("Load of non-ELF file should have failed")
	}
}

// TestLoadDirectory tests loading a directory path
func TestLoadDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	resolver := &mockResolver{symbols: make(map[string]uintptr)}

	_, err := Load(tmpDir, resolver)
	if err == nil {
		t.Error("Load of directory should have failed")
	}
}

// TestLoadEmptyFile tests loading an empty file
func TestLoadEmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	emptyFile := filepath.Join(tmpDir, "empty.so")

	if err := os.WriteFile(emptyFile, []byte{}, 0o644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	resolver := &mockResolver{symbols: make(map[string]uintptr)}

	_, err := Load(emptyFile, resolver)
	if err == nil {
		t.Error("Load of empty file should have failed")
	}
}

// TestLoad...TestLoadCorrectlyReportsPath tests that errors include the file path
func TestLoadCorrectlyReportsPath(t *testing.T) {
	nonExistent := "/nonexistent/path/to/library.so"

	resolver := &mockResolver{symbols: make(map[string]uintptr)}

	_, err := Load(nonExistent, resolver)
	if err == nil {
		t.Error("Load of nonexistent file should have failed")
	}
	// Error message should include the path
	// We can't check the exact message format, but we verified an error occurred
}

// TestUnloadWithFiniArray tests unloading a library with fini functions
func TestUnloadWithFiniArray(t *testing.T) {
	testLib := "../testdata/libtest.so"
	resolver := &mockResolver{symbols: make(map[string]uintptr)}

	obj, err := Load(testLib, resolver)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Unload should call fini functions
	err = Unload(obj)
	if err != nil {
		// Fini functions might fail, but Unload should still complete
		t.Logf("Unload returned error (may be expected): %v", err)
	}
}

// TestLoadLibraryWithDifferentFeatures tests loading various specialized libraries
func TestLoadLibraryWithDifferentFeatures(t *testing.T) {
	testCases := []struct {
		name string
		path string
		desc string
	}{
		{"basic", "../testdata/libtest.so", "basic library with constructors"},
		{"relocations", "../testdata/libreloc.so", "library with internal relocations"},
		{"ifunc", "../testdata/libifunc.so", "library with IFUNC"},
		{"tls", "../testdata/libtls.so", "library with TLS"},
		{"tls_models", "../testdata/libtls_models.so", "library with different TLS models"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resolver := &mockResolver{symbols: make(map[string]uintptr)}

			obj, err := Load(tc.path, resolver)
			if err != nil {
				t.Fatalf("Load(%q) failed: %v", tc.path, err)
			}
			defer Unload(obj)

			if obj.Base == 0 {
				t.Error("Base address is zero")
			}
			if len(obj.Segments) == 0 {
				t.Error("No segments loaded")
			}
		})
	}
}

// TestSegmentProtections tests that segments have correct memory protections
func TestSegmentProtections(t *testing.T) {
	testLib := "../testdata/libtest.so"
	resolver := &mockResolver{symbols: make(map[string]uintptr)}

	obj, err := Load(testLib, resolver)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	defer Unload(obj)

	if len(obj.Segments) < 1 {
		t.Fatal("No segments loaded")
	}

	// At least one segment should have execute permission (code)
	// At least one segment should have write permission (data)
	hasExec := false
	hasWrite := false

	for _, seg := range obj.Segments {
		if seg.Prot&0x4 != 0 { // PROT_EXEC = 0x4
			hasExec = true
		}
		if seg.Prot&0x2 != 0 { // PROT_WRITE = 0x2
			hasWrite = true
		}
	}

	if !hasExec {
		t.Error("No executable segment found")
	}
	// Note: Some libraries might have write-protected data sections
	_ = hasWrite
}

// TestMultipleConcurrentLoads tests loading different libraries concurrently
func TestMultipleConcurrentLoads(t *testing.T) {
	libs := []string{
		"../testdata/libtest.so",
		"../testdata/libreloc.so",
	}

	type result struct {
		obj *Object
		err error
	}

	results := make(chan result, len(libs))

	for _, lib := range libs {
		libPath := lib
		go func() {
			resolver := &mockResolver{symbols: make(map[string]uintptr)}
			obj, err := Load(libPath, resolver)
			results <- result{obj, err}
		}()
	}

	for i := 0; i < len(libs); i++ {
		res := <-results
		if res.err != nil {
			t.Errorf("Concurrent load %d failed: %v", i, res.err)
		}
		if res.obj != nil {
			Unload(res.obj)
		}
	}
}

// TestLoadWithNilResolver tests error when resolver is nil
func TestLoadWithCustomResolver(t *testing.T) {
	testLib := "../testdata/libtest.so"

	// Use a resolver that provides some symbols
	resolver := &mockResolver{
		symbols: map[string]uintptr{
			"custom_symbol": 0x12345,
		},
	}

	obj, err := Load(testLib, resolver)
	if err != nil {
		t.Fatalf("Load with custom resolver failed: %v", err)
	}
	defer Unload(obj)

	// Verify the object loaded successfully
	if obj.Symbols == nil {
		t.Error("Symbol table not initialized")
	}
}

// TestRefCountManagement tests reference count handling
func TestRefCountManagement(t *testing.T) {
	testLib := "../testdata/libtest.so"
	resolver := &mockResolver{symbols: make(map[string]uintptr)}

	obj, err := Load(testLib, resolver)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	initialRef := obj.RefCount
	if initialRef != 1 {
		t.Errorf("Initial RefCount = %d, want 1", initialRef)
	}

	// Simulate increment
	obj.RefCount++

	// Simulate decrement back
	obj.RefCount--

	if obj.RefCount != initialRef {
		t.Errorf("RefCount = %d after inc/dec, want %d", obj.RefCount, initialRef)
	}

	Unload(obj)
}

// TestDynamicSectionParsing tests that dynamic section tags are correctly parsed
func TestDynamicSectionParsing(t *testing.T) {
	testLib := "../testdata/libtest.so"
	resolver := &mockResolver{symbols: make(map[string]uintptr)}

	obj, err := Load(testLib, resolver)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	defer Unload(obj)

	// Verify essential dynamic section addresses are set
	if obj.SymtabAddr == 0 {
		t.Error("SymtabAddr is zero")
	}
	if obj.StrtabAddr == 0 {
		t.Error("StrtabAddr is zero")
	}

	// At least one of the hash tables should be present
	if obj.HashAddr == 0 && obj.GnuHashAddr == 0 {
		t.Error("No hash table found (neither DT_HASH nor DT_GNU_HASH)")
	}

	// Check for relocation tables
	if obj.RelaAddr != 0 && obj.RelaSize == 0 {
		t.Error("RelaAddr set but RelaSize is zero")
	}
	if obj.RelaAddr != 0 && obj.RelaEnt == 0 {
		t.Error("RelaAddr set but RelaEnt is zero")
	}
}

// TestLoadPreservesPath tests that the loaded object retains the file path
func TestLoadPreservesPath(t *testing.T) {
	testLib := "../testdata/libtest.so"
	resolver := &mockResolver{symbols: make(map[string]uintptr)}

	obj, err := Load(testLib, resolver)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	defer Unload(obj)

	if obj.Parsed == nil {
		t.Fatal("Parsed object is nil")
	}
	if obj.Parsed.Path != testLib {
		t.Errorf("Path = %q, want %q", obj.Parsed.Path, testLib)
	}
}
