//go:build !race

package loader

import (
	"strings"
	"testing"
	"unsafe"
)

// TestRelocationOffsetBoundsChecking tests that out-of-bounds relocation offsets are rejected.
// This test is excluded from race builds because it intentionally uses malformed data that
// triggers checkptr violations in the race detector before our validation logic runs.
// The actual bounds checking code is correct; this is purely a testing limitation.
func TestRelocationOffsetBoundsChecking(t *testing.T) {
	testLib := "../testdata/libtest.so"
	resolver := &mockResolver{symbols: make(map[string]uintptr)}

	obj, err := Load(testLib, resolver)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	defer Unload(obj)

	baseVAddr := obj.Parsed.BaseVAddr
	memSize := obj.Parsed.MemSize

	// Test case 1: Offset before BaseVAddr
	beforeBaseOffset := baseVAddr - 1
	
	// Test case 2: Offset at exact upper boundary (should fail - must be strictly less)
	atUpperBound := baseVAddr + memSize
	
	// Test case 3: Offset beyond upper boundary
	beyondUpperBound := baseVAddr + memSize + 0x1000

	testCases := []struct {
		name   string
		offset uint64
	}{
		{"offset before base", beforeBaseOffset},
		{"offset at upper bound", atUpperBound},
		{"offset beyond upper bound", beyondUpperBound},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a single relocation entry at an invalid offset
			relEntries := make([]relaEntry, 1)
			relEntries[0] = relaEntry{
				Offset: tc.offset,
				Info:   0, // R_X86_64_NONE or similar
				Addend: 0,
			}

			tableAddr := uintptr(unsafe.Pointer(&relEntries[0]))
			tableSize := uint64(24) // One relaEntry

			err := applyRelaTable(obj, tableAddr, tableSize, resolver)
			if err == nil {
				t.Errorf("applyRelaTable with offset %#x should have failed (base=%#x, memSize=%#x)",
					tc.offset, baseVAddr, memSize)
			} else if !strings.Contains(err.Error(), "out of range") {
				t.Errorf("applyRelaTable error should mention 'out of range', got: %v", err)
			}
		})
	}
}
