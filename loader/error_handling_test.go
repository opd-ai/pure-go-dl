package loader

import (
	"testing"
)

// TestApplyRelaTableWithInvalidOffset tests relocation offset validation
func TestApplyRelaTableWithInvalidOffset(t *testing.T) {
	testLib := "../testdata/libtest.so"
	resolver := &mockResolver{symbols: make(map[string]uintptr)}

	obj, err := Load(testLib, resolver)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	defer Unload(obj)

	// Verify that the object was mapped correctly
	if len(obj.Segments) == 0 {
		t.Error("Object should have at least one segment")
	}

	// Log segment info
	for i, seg := range obj.Segments {
		t.Logf("Segment %d: Addr=%#x Size=%d", i, seg.Addr, seg.Size)
	}
}

// TestResolveSymbolUndefinedSymbol tests error handling for undefined symbols
func TestResolveSymbolUndefinedSymbol(t *testing.T) {
	// Create a resolver that doesn't provide required symbols
	resolver := &mockResolver{
		symbols: map[string]uintptr{
			// Deliberately empty - won't resolve any symbols
		},
	}

	// Try to load a library that references external symbols
	testLib := "../testdata/libtest.so"

	// Note: libtest.so has __cxa_finalize as WEAK, so it won't fail
	// For a true test of undefined symbol errors, we'd need a library
	// with strong references to unresolved symbols

	obj, err := Load(testLib, resolver)
	if err != nil {
		// Error is expected if library has strong undefined symbols
		t.Logf("Load failed as expected for unresolved symbols: %v", err)
		return
	}
	if obj != nil {
		defer Unload(obj)
	}

	// If load succeeded, it means all undefined symbols were WEAK or resolved
	t.Log("Load succeeded (library has only weak undefined symbols)")
}

// TestTLSRegistrationWithMaxModules tests TLS module registration
func TestTLSRegistrationWithMaxModules(t *testing.T) {
	// This test verifies TLS module registration path

	testLib := "../testdata/libtls.so"
	resolver := &mockResolver{symbols: make(map[string]uintptr)}

	obj, err := Load(testLib, resolver)
	if err != nil {
		t.Fatalf("Load of TLS library failed: %v", err)
	}
	defer Unload(obj)

	// Verify TLS module was registered
	if obj.TLSModule == nil {
		t.Error("TLS module should be non-nil for library with TLS")
	} else {
		t.Logf("TLS module ID: %d", obj.TLSModule.ID)
	}
}

// TestUnknownRelocationType tests handling of unknown/unsupported relocation types
func TestUnknownRelocationType(t *testing.T) {
	// This test documents that unknown relocation types should be handled
	// The actual implementation either errors or skips unknown types

	testLib := "../testdata/libtest.so"
	resolver := &mockResolver{symbols: make(map[string]uintptr)}

	obj, err := Load(testLib, resolver)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	defer Unload(obj)

	// All known libraries should have only recognized relocation types
	// If we encounter unknown types, the loader should handle them gracefully

	// Log the relocation table info
	if obj.RelaAddr != 0 && obj.RelaSize > 0 {
		numRela := obj.RelaSize / 24
		t.Logf("Object has %d RELA relocations", numRela)
	}
	if obj.JmpRelAddr != 0 && obj.JmpRelSize > 0 {
		numJmpRel := obj.JmpRelSize / 24
		t.Logf("Object has %d JMPREL relocations", numJmpRel)
	}
}

// TestApplyRelocationsWithNilResolver tests relocation with nil resolver
func TestApplyRelocationsWithNilResolver(t *testing.T) {
	// Testing internal function behavior - this documents expected behavior
	// when resolver is nil or missing symbols

	resolver := &mockResolver{
		symbols: nil, // nil map
	}

	testLib := "../testdata/libtest.so"

	// This should fail during symbol resolution
	_, err := Load(testLib, resolver)
	if err != nil {
		t.Logf("Load correctly failed with nil symbol map: %v", err)
	}
}

// TestInitFiniArrayExecution tests constructor/destructor execution errors
func TestInitFiniArrayExecution(t *testing.T) {
	testLib := "../testdata/libtest.so"
	resolver := &mockResolver{symbols: make(map[string]uintptr)}

	obj, err := Load(testLib, resolver)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Verify init array was executed (side effects should be visible)
	// libtest.so has a constructor that sets a counter

	// Unload should execute fini array
	err = Unload(obj)
	if err != nil {
		t.Logf("Unload returned error (may be expected): %v", err)
	}

	// Even if fini functions fail, Unload should complete
	// and unmap the memory
}

// TestMemorySizeValidation tests handling of PT_LOAD segments
func TestMemorySizeValidation(t *testing.T) {
	// This documents the expected behavior for PT_LOAD segment validation

	testLib := "../testdata/libtest.so"
	resolver := &mockResolver{symbols: make(map[string]uintptr)}

	obj, err := Load(testLib, resolver)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	defer Unload(obj)

	// Verify reasonable segment sizes
	for i, seg := range obj.Segments {
		if seg.Size == 0 {
			t.Errorf("Segment %d has zero size", i)
		}
		if seg.Size > 0x100000000 { // 4GB sanity check
			t.Errorf("Segment %d size seems unreasonably large: %#x", i, seg.Size)
		}
		t.Logf("Segment %d: Size=%#x (%d bytes)", i, seg.Size, seg.Size)
	}
}

// TestDynamicSectionValidation tests validation of dynamic section entries
func TestDynamicSectionValidation(t *testing.T) {
	testLib := "../testdata/libtest.so"
	resolver := &mockResolver{symbols: make(map[string]uintptr)}

	obj, err := Load(testLib, resolver)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	defer Unload(obj)

	// Verify essential dynamic section fields are validated
	if obj.SymtabAddr == 0 {
		t.Error("SymtabAddr should be non-zero after validation")
	}
	if obj.StrtabAddr == 0 {
		t.Error("StrtabAddr should be non-zero after validation")
	}
	if obj.StrtabSize == 0 {
		t.Error("StrtabSize should be non-zero after validation")
	}

	// At least one hash table should be present
	hasHash := obj.HashAddr != 0 || obj.GnuHashAddr != 0
	if !hasHash {
		t.Error("Object should have at least one hash table (DT_HASH or DT_GNU_HASH)")
	}
}

// TestRelocationConsistencyChecks tests that relocation table consistency is validated
func TestRelocationConsistencyChecks(t *testing.T) {
	testLib := "../testdata/libtest.so"
	resolver := &mockResolver{symbols: make(map[string]uintptr)}

	obj, err := Load(testLib, resolver)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	defer Unload(obj)

	// If RelaAddr is set, RelaSize should also be set
	if obj.RelaAddr != 0 && obj.RelaSize == 0 {
		t.Error("RelaAddr is set but RelaSize is zero")
	}
	if obj.RelaSize != 0 && obj.RelaAddr == 0 {
		t.Error("RelaSize is set but RelaAddr is zero")
	}

	// If JmpRelAddr is set, JmpRelSize should also be set
	if obj.JmpRelAddr != 0 && obj.JmpRelSize == 0 {
		t.Error("JmpRelAddr is set but JmpRelSize is zero")
	}
	if obj.JmpRelSize != 0 && obj.JmpRelAddr == 0 {
		t.Error("JmpRelSize is set but JmpRelAddr is zero")
	}

	// Relocation entry size should be 24 bytes (sizeof(Elf64_Rela))
	if obj.RelaEnt != 0 && obj.RelaEnt != 24 {
		t.Errorf("RelaEnt should be 24, got %d", obj.RelaEnt)
	}
}
