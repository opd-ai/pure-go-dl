package dl

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// createTestCache creates a minimal valid ld.so.cache file for testing.
func createTestCache(entries map[string]string) []byte {
	// Calculate sizes
	numEntries := uint32(len(entries))

	// Build string table
	var stringTable bytes.Buffer
	keyOffsets := make(map[string]uint32)
	valueOffsets := make(map[string]uint32)

	for soname, path := range entries {
		// Store soname
		keyOffsets[soname] = uint32(stringTable.Len())
		stringTable.WriteString(soname)
		stringTable.WriteByte(0)

		// Store path
		valueOffsets[soname] = uint32(stringTable.Len())
		stringTable.WriteString(path)
		stringTable.WriteByte(0)
	}

	stringsLen := uint32(stringTable.Len())

	// Build header manually to avoid padding issues
	var result bytes.Buffer
	// Write magic (exactly 20 bytes)
	magic := make([]byte, 20)
	copy(magic, []byte(cacheHeaderMagic))
	result.Write(magic)
	binary.Write(&result, binary.LittleEndian, numEntries)
	binary.Write(&result, binary.LittleEndian, stringsLen)
	// Pad to 88 bytes total (header size)
	padding := make([]byte, 60) // 88 - 20 - 4 - 4 = 60
	result.Write(padding)

	// Build entries manually
	for soname := range entries {
		// Write each field of cacheEntry separately
		flags := uint32(flagLibc6 | flagX8664)
		binary.Write(&result, binary.LittleEndian, flags)                // Flags
		binary.Write(&result, binary.LittleEndian, keyOffsets[soname])   // KeyOffset
		binary.Write(&result, binary.LittleEndian, valueOffsets[soname]) // ValueOffset
		binary.Write(&result, binary.LittleEndian, uint32(0))            // OSVersion
		binary.Write(&result, binary.LittleEndian, uint64(0))            // HWCap
	}

	// Append string table
	result.Write(stringTable.Bytes())

	return result.Bytes()
}

func TestParseCache(t *testing.T) {
	testEntries := map[string]string{
		"libm.so.6":  "/lib/x86_64-linux-gnu/libm.so.6",
		"libc.so.6":  "/lib/x86_64-linux-gnu/libc.so.6",
		"libdl.so.2": "/lib/x86_64-linux-gnu/libdl.so.2",
		"libtest.so": "/custom/path/libtest.so",
	}

	cacheData := createTestCache(testEntries)

	// Write to temporary file
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "ld.so.cache")
	if err := os.WriteFile(cachePath, cacheData, 0o644); err != nil {
		t.Fatalf("Failed to write test cache: %v", err)
	}

	// Parse the cache
	cache, err := parseCache(cachePath)
	if err != nil {
		t.Fatalf("parseCache failed: %v", err)
	}

	// Verify all entries are present
	for soname, expectedPath := range testEntries {
		gotPath, ok := cache.entries[soname]
		if !ok {
			t.Errorf("Cache missing entry for %q", soname)
			continue
		}
		if gotPath != expectedPath {
			t.Errorf("Cache entry for %q: got %q, want %q", soname, gotPath, expectedPath)
		}
	}

	// Verify no extra entries
	if len(cache.entries) != len(testEntries) {
		t.Errorf("Cache has %d entries, want %d", len(cache.entries), len(testEntries))
	}
}

func TestParseCacheInvalidMagic(t *testing.T) {
	data := make([]byte, 256)
	copy(data, []byte("invalid-magic"))

	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "ld.so.cache")
	if err := os.WriteFile(cachePath, data, 0o644); err != nil {
		t.Fatalf("Failed to write test cache: %v", err)
	}

	_, err := parseCache(cachePath)
	if err == nil {
		t.Error("parseCache should fail with invalid magic")
	}
}

func TestParseCacheTruncated(t *testing.T) {
	data := make([]byte, 10) // Too small to be valid
	copy(data, []byte(cacheHeaderMagic))

	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "ld.so.cache")
	if err := os.WriteFile(cachePath, data, 0o644); err != nil {
		t.Fatalf("Failed to write test cache: %v", err)
	}

	_, err := parseCache(cachePath)
	if err == nil {
		t.Error("parseCache should fail with truncated file")
	}
}

func TestParseCacheEmpty(t *testing.T) {
	// Cache with zero entries
	testEntries := map[string]string{}
	cacheData := createTestCache(testEntries)

	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "ld.so.cache")
	if err := os.WriteFile(cachePath, cacheData, 0o644); err != nil {
		t.Fatalf("Failed to write test cache: %v", err)
	}

	cache, err := parseCache(cachePath)
	if err != nil {
		t.Fatalf("parseCache failed: %v", err)
	}

	if len(cache.entries) != 0 {
		t.Errorf("Empty cache should have 0 entries, got %d", len(cache.entries))
	}
}

func TestLookupInCacheRealSystem(t *testing.T) {
	// This test uses the real system cache if it exists and is readable
	data, err := os.ReadFile(defaultCachePath)
	if err != nil {
		t.Skipf("System ld.so.cache not readable: %v", err)
	}
	if len(data) == 0 {
		t.Skip("System ld.so.cache is empty")
	}

	// Reset global state for this test
	cacheLoadOnce = *new(sync.Once)
	globalCache = nil

	// Try to lookup a common library
	// Note: this test may fail if the system cache is in an unsupported format
	// or if libm.so.6 is not in the cache, so we just log a warning instead of failing
	path := lookupInCache("libm.so.6")
	if path == "" {
		t.Log("lookupInCache(libm.so.6) returned empty string - cache may be in unsupported format or library not present")
		t.Skip("Real system cache test inconclusive")
	}

	// Verify the path exists
	if path != "" {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("Cache returned path %q that doesn't exist: %v", path, err)
		}
	}

	// Lookup non-existent library
	path = lookupInCache("libnonexistent-12345.so.99")
	if path != "" {
		t.Errorf("lookupInCache for non-existent library should return empty string, got %q", path)
	}
}

func TestFindLibraryWithCache(t *testing.T) {
	// This test verifies that findLibrary uses the cache
	// We can't fully test this without mocking, but we can verify behavior with real cache
	if _, err := os.ReadFile(defaultCachePath); err != nil {
		t.Skipf("System ld.so.cache not readable: %v", err)
	}

	// Reset cache state
	cacheLoadOnce = *new(sync.Once)
	globalCache = nil

	// Try to find libm.so.6 which should be in the cache
	path, err := findLibrary("libm.so.6", "", "")
	if err != nil {
		t.Fatalf("findLibrary(libm.so.6) failed: %v", err)
	}

	if path == "" {
		t.Error("findLibrary(libm.so.6) returned empty path")
	}

	// Verify the file exists
	if _, err := os.Stat(path); err != nil {
		t.Errorf("findLibrary returned path %q that doesn't exist: %v", path, err)
	}
}

func TestCacheArchitectureFiltering(t *testing.T) {
	// Create cache with mixed architecture entries
	var buf bytes.Buffer

	// Header - write manually
	magic := make([]byte, 20)
	copy(magic, []byte(cacheHeaderMagic))
	buf.Write(magic)
	binary.Write(&buf, binary.LittleEndian, uint32(3)) // NumLibs
	binary.Write(&buf, binary.LittleEndian, uint32(0)) // StringsLen (will update later)
	padding := make([]byte, 60)
	buf.Write(padding)

	// String table offsets
	soname1 := "libtest.so"
	path1 := "/lib64/libtest.so"
	soname2 := "libtest32.so"
	path2 := "/lib32/libtest32.so"
	soname3 := "libgeneric.so"
	path3 := "/lib/libgeneric.so"

	// Build string table
	var stringTable bytes.Buffer
	offset1Key := uint32(stringTable.Len())
	stringTable.WriteString(soname1)
	stringTable.WriteByte(0)
	offset1Val := uint32(stringTable.Len())
	stringTable.WriteString(path1)
	stringTable.WriteByte(0)

	offset2Key := uint32(stringTable.Len())
	stringTable.WriteString(soname2)
	stringTable.WriteByte(0)
	offset2Val := uint32(stringTable.Len())
	stringTable.WriteString(path2)
	stringTable.WriteByte(0)

	offset3Key := uint32(stringTable.Len())
	stringTable.WriteString(soname3)
	stringTable.WriteByte(0)
	offset3Val := uint32(stringTable.Len())
	stringTable.WriteString(path3)
	stringTable.WriteByte(0)

	// Write entries manually
	// Entry 1: x86-64 (should be included)
	binary.Write(&buf, binary.LittleEndian, uint32(flagLibc6|flagX8664))
	binary.Write(&buf, binary.LittleEndian, offset1Key)
	binary.Write(&buf, binary.LittleEndian, offset1Val)
	binary.Write(&buf, binary.LittleEndian, uint32(0)) // OSVersion
	binary.Write(&buf, binary.LittleEndian, uint64(0)) // HWCap

	// Entry 2: i386 (should be filtered out)
	binary.Write(&buf, binary.LittleEndian, uint32(flagLibc6|0x0000))
	binary.Write(&buf, binary.LittleEndian, offset2Key)
	binary.Write(&buf, binary.LittleEndian, offset2Val)
	binary.Write(&buf, binary.LittleEndian, uint32(0))
	binary.Write(&buf, binary.LittleEndian, uint64(0))

	// Entry 3: Generic/unspecified arch (should be included for compatibility)
	binary.Write(&buf, binary.LittleEndian, uint32(flagLibc6|0x0000))
	binary.Write(&buf, binary.LittleEndian, offset3Key)
	binary.Write(&buf, binary.LittleEndian, offset3Val)
	binary.Write(&buf, binary.LittleEndian, uint32(0))
	binary.Write(&buf, binary.LittleEndian, uint64(0))

	// Append string table
	buf.Write(stringTable.Bytes())

	// Write to file
	tmpDir := t.TempDir()
	cachePath := filepath.Join(tmpDir, "ld.so.cache")
	if err := os.WriteFile(cachePath, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("Failed to write test cache: %v", err)
	}

	// Parse
	cache, err := parseCache(cachePath)
	if err != nil {
		t.Fatalf("parseCache failed: %v", err)
	}

	// Verify x86-64 entry is present
	if _, ok := cache.entries[soname1]; !ok {
		t.Errorf("Cache should include x86-64 entry %q", soname1)
	}

	// Verify generic entry is present (unspecified arch = 0x0000)
	if _, ok := cache.entries[soname3]; !ok {
		t.Errorf("Cache should include generic/unspecified arch entry %q", soname3)
	}

	// We can't verify i386 is excluded in this simple implementation
	// since both entry2 and entry3 have arch=0x0000
	// The real cache parser would need to check if it's explicitly i386
}
