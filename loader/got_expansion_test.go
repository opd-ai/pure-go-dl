package loader

import (
	"testing"
	"unsafe"
)

// TestGOTExpansion_MultiplePages verifies that the GOT can expand beyond
// the initial 4096-byte page to accommodate libraries with many TLS symbols.
// This addresses IMPROVEMENT_FINDINGS issue 1.6 (GOT Size Exhaustion).
func TestGOTExpansion_MultiplePages(t *testing.T) {
	testLib := "../testdata/libtest.so"
	resolver := &mockResolver{symbols: make(map[string]uintptr)}

	obj, err := Load(testLib, resolver)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	defer Unload(obj)

	// Simulate allocation of 300 TLS symbol pairs (600 uint64 entries = 4800 bytes).
	// This requires at least 2 GOT pages (each page is 4096 bytes).
	const numSymbols = 300
	allocatedAddrs := make(map[uint32]uintptr)

	for i := uint32(0); i < numSymbols; i++ {
		addr, err := allocateGOTEntryPair(obj, i)
		if err != nil {
			t.Fatalf("Failed to allocate GOT entry for symbol %d: %v", i, err)
		}

		if addr == 0 {
			t.Fatalf("allocateGOTEntryPair returned zero address for symbol %d", i)
		}

		// Verify that the address is unique.
		for symIdx, prevAddr := range allocatedAddrs {
			if addr == prevAddr {
				t.Fatalf("Duplicate GOT address %#x for symbols %d and %d", addr, symIdx, i)
			}
		}
		allocatedAddrs[i] = addr
	}

	// Verify that at least 2 pages were allocated.
	expectedPages := (numSymbols*16 + 4095) / 4096 // Round up to page count
	if len(obj.GOTPages) < expectedPages {
		t.Errorf("Expected at least %d GOT pages, got %d", expectedPages, len(obj.GOTPages))
	}

	t.Logf("Successfully allocated %d GOT entries across %d pages", numSymbols, len(obj.GOTPages))

	// Verify that entries are accessible and writable.
	// Write a sentinel value to each pair.
	for i := uint32(0); i < numSymbols; i++ {
		addr := allocatedAddrs[i]
		// Write module ID to first uint64, offset to second uint64.
		*(*uint64)(unsafe.Pointer(addr)) = uint64(i) + 0x1000
		*(*uint64)(unsafe.Pointer(addr + 8)) = uint64(i) + 0x2000
	}

	// Read back and verify.
	for i := uint32(0); i < numSymbols; i++ {
		addr := allocatedAddrs[i]
		moduleID := *(*uint64)(unsafe.Pointer(addr))
		offset := *(*uint64)(unsafe.Pointer(addr + 8))

		expectedModuleID := uint64(i) + 0x1000
		expectedOffset := uint64(i) + 0x2000

		if moduleID != expectedModuleID {
			t.Errorf("Symbol %d: expected moduleID %#x, got %#x", i, expectedModuleID, moduleID)
		}
		if offset != expectedOffset {
			t.Errorf("Symbol %d: expected offset %#x, got %#x", i, expectedOffset, offset)
		}
	}
}

// TestGOTExpansion_IdempotentAllocation verifies that requesting the same
// symbol twice returns the same GOT address.
func TestGOTExpansion_IdempotentAllocation(t *testing.T) {
	testLib := "../testdata/libtest.so"
	resolver := &mockResolver{symbols: make(map[string]uintptr)}

	obj, err := Load(testLib, resolver)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	defer Unload(obj)

	const symIdx = uint32(42)

	addr1, err := allocateGOTEntryPair(obj, symIdx)
	if err != nil {
		t.Fatalf("First allocation failed: %v", err)
	}

	addr2, err := allocateGOTEntryPair(obj, symIdx)
	if err != nil {
		t.Fatalf("Second allocation failed: %v", err)
	}

	if addr1 != addr2 {
		t.Errorf("Duplicate allocation returned different addresses: %#x != %#x", addr1, addr2)
	}

	// GOTSize should reflect only one entry pair (16 bytes).
	if obj.GOTSize != 16 {
		t.Errorf("Expected GOTSize=16 after idempotent allocation, got %d", obj.GOTSize)
	}
}

// TestGOTExpansion_PageBoundary verifies that allocations crossing page
// boundaries work correctly.
func TestGOTExpansion_PageBoundary(t *testing.T) {
	testLib := "../testdata/libtest.so"
	resolver := &mockResolver{symbols: make(map[string]uintptr)}

	obj, err := Load(testLib, resolver)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	defer Unload(obj)

	// Allocate exactly 256 entries (4096 bytes), filling the first page.
	const numSymbolsPage1 = 256
	for i := uint32(0); i < numSymbolsPage1; i++ {
		_, err := allocateGOTEntryPair(obj, i)
		if err != nil {
			t.Fatalf("Failed to allocate entry %d: %v", i, err)
		}
	}

	// Verify we're using exactly 1 page.
	if len(obj.GOTPages) != 1 {
		t.Errorf("Expected 1 page after filling first page, got %d", len(obj.GOTPages))
	}

	// Allocate one more entry, which should trigger a new page.
	addr, err := allocateGOTEntryPair(obj, numSymbolsPage1)
	if err != nil {
		t.Fatalf("Failed to allocate boundary-crossing entry: %v", err)
	}

	// Verify a second page was allocated.
	if len(obj.GOTPages) != 2 {
		t.Errorf("Expected 2 pages after boundary crossing, got %d", len(obj.GOTPages))
	}

	// The new entry should be at the start of the second page.
	expectedAddr := obj.GOTPages[1]
	if addr != expectedAddr {
		t.Errorf("Expected entry at start of page 2 (%#x), got %#x", expectedAddr, addr)
	}

	t.Logf("Successfully allocated across page boundary: %d pages allocated", len(obj.GOTPages))
}
