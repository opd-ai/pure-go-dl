package loader

import (
	"debug/elf"
	"os"
	"syscall"
	"testing"
	"unsafe"

	"github.com/opd-ai/pure-go-dl/internal/mmap"
	"golang.org/x/sys/unix"
)

func TestElfProt(t *testing.T) {
	tests := []struct {
		flags elf.ProgFlag
		want  string // description of expected protection
	}{
		{0, "PROT_NONE"},
		{elf.PF_R, "PROT_READ"},
		{elf.PF_W, "PROT_WRITE"},
		{elf.PF_X, "PROT_EXEC"},
		{elf.PF_R | elf.PF_W, "PROT_READ|PROT_WRITE"},
		{elf.PF_R | elf.PF_X, "PROT_READ|PROT_EXEC"},
		{elf.PF_R | elf.PF_W | elf.PF_X, "PROT_READ|PROT_WRITE|PROT_EXEC"},
	}

	for _, tt := range tests {
		got := elfProt(tt.flags)
		// Verify the bits match expectations
		hasRead := (got & unix.PROT_READ) != 0
		hasWrite := (got & unix.PROT_WRITE) != 0
		hasExec := (got & unix.PROT_EXEC) != 0

		wantRead := (tt.flags & elf.PF_R) != 0
		wantWrite := (tt.flags & elf.PF_W) != 0
		wantExec := (tt.flags & elf.PF_X) != 0

		if hasRead != wantRead || hasWrite != wantWrite || hasExec != wantExec {
			t.Errorf("elfProt(%v) = 0x%x (%s), want %s", tt.flags, got, protToString(got), tt.want)
		}
	}
}

func protToString(prot int) string {
	if prot == 0 {
		return "PROT_NONE"
	}
	var parts []string
	if prot&unix.PROT_READ != 0 {
		parts = append(parts, "PROT_READ")
	}
	if prot&unix.PROT_WRITE != 0 {
		parts = append(parts, "PROT_WRITE")
	}
	if prot&unix.PROT_EXEC != 0 {
		parts = append(parts, "PROT_EXEC")
	}
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += "|"
		}
		result += p
	}
	return result
}

func TestPageDown(t *testing.T) {
	pageSize := uint64(unix.Getpagesize())
	tests := []struct {
		input uint64
		check func(uint64, uint64) bool
	}{
		{0, func(got, ps uint64) bool { return got == 0 }},
		{1, func(got, ps uint64) bool { return got == 0 }},
		{pageSize - 1, func(got, ps uint64) bool { return got == 0 }},
		{pageSize, func(got, ps uint64) bool { return got == ps }},
		{pageSize + 1, func(got, ps uint64) bool { return got == ps }},
		{2 * pageSize, func(got, ps uint64) bool { return got == 2*ps }},
		{2*pageSize + 100, func(got, ps uint64) bool { return got == 2*ps }},
	}

	for _, tt := range tests {
		got := pageDown(tt.input)
		if !tt.check(got, pageSize) {
			t.Errorf("pageDown(%d) = %d (pageSize=%d)", tt.input, got, pageSize)
		}
		// Verify result is page-aligned
		if got%pageSize != 0 {
			t.Errorf("pageDown(%d) = %d, not page-aligned", tt.input, got)
		}
	}
}

func TestPageUp(t *testing.T) {
	pageSize := uint64(unix.Getpagesize())
	tests := []struct {
		input uint64
		check func(uint64, uint64) bool
	}{
		{0, func(got, ps uint64) bool { return got == 0 }},
		{1, func(got, ps uint64) bool { return got == ps }},
		{pageSize - 1, func(got, ps uint64) bool { return got == ps }},
		{pageSize, func(got, ps uint64) bool { return got == ps }},
		{pageSize + 1, func(got, ps uint64) bool { return got == 2*ps }},
		{2 * pageSize, func(got, ps uint64) bool { return got == 2*ps }},
	}

	for _, tt := range tests {
		got := pageUp(tt.input)
		if !tt.check(got, pageSize) {
			t.Errorf("pageUp(%d) = %d (pageSize=%d)", tt.input, got, pageSize)
		}
		// Verify result is page-aligned
		if got%pageSize != 0 {
			t.Errorf("pageUp(%d) = %d, not page-aligned", tt.input, got)
		}
	}
}

func TestZeroMem(t *testing.T) {
	// Use mmap'd memory like the real code does, to avoid checkptr violations
	size := uintptr(4096) // One page
	addr, err := mmap.Map(0, size, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_PRIVATE|syscall.MAP_ANONYMOUS, -1, 0)
	if err != nil {
		t.Fatalf("mmap failed: %v", err)
	}
	defer mmap.Unmap(addr, size)

	// Create a slice view of the mmap'd region
	buf := unsafe.Slice((*byte)(unsafe.Pointer(addr)), size)

	// Fill with non-zero data
	for i := range buf {
		buf[i] = byte((i % 255) + 1) // Start at 1 to avoid accidental zeros
	}

	// Zero a portion in the middle
	zeroStart := uintptr(10)
	zeroCount := uintptr(50)
	zeroMem(addr+zeroStart, zeroCount)

	// Check that the region was zeroed
	for i := 10; i < 60; i++ {
		if buf[i] != 0 {
			t.Errorf("buf[%d] = %d after zeroMem, want 0", i, buf[i])
		}
	}

	// Check that regions outside weren't touched
	for i := 0; i < 10; i++ {
		expected := byte((i % 255) + 1)
		if buf[i] != expected {
			t.Errorf("buf[%d] = %d, want %d (should not be modified)", i, buf[i], expected)
		}
	}
	for i := 60; i < 100; i++ {
		expected := byte((i % 255) + 1)
		if buf[i] != expected {
			t.Errorf("buf[%d] = %d, want %d (should not be modified)", i, buf[i], expected)
		}
	}
}

// mockResolver is a simple SymbolResolver for testing
type mockResolver struct {
	symbols map[string]uintptr
}

func (m *mockResolver) Resolve(name string) (uintptr, error) {
	if addr, ok := m.symbols[name]; ok {
		return addr, nil
	}
	return 0, nil
}

func TestLoad(t *testing.T) {
	// Test loading the test library
	testLib := "../testdata/libtest.so"

	resolver := &mockResolver{
		symbols: make(map[string]uintptr),
	}

	obj, err := Load(testLib, resolver)
	if err != nil {
		t.Fatalf("Load(%q) failed: %v", testLib, err)
	}
	defer Unload(obj)

	// Verify object structure
	if obj == nil {
		t.Fatal("Load returned nil object")
	}
	if obj.Base == 0 {
		t.Error("obj.Base is zero")
	}
	if len(obj.Segments) == 0 {
		t.Error("obj.Segments is empty")
	}
	if obj.Symbols == nil {
		t.Error("obj.Symbols is nil")
	}
	if obj.RefCount != 1 {
		t.Errorf("obj.RefCount = %d, want 1", obj.RefCount)
	}

	// Verify dynamic addresses were computed
	if obj.SymtabAddr == 0 {
		t.Error("obj.SymtabAddr is zero")
	}
	if obj.StrtabAddr == 0 {
		t.Error("obj.StrtabAddr is zero")
	}

	// Either SysV hash or GNU hash should be present
	if obj.HashAddr == 0 && obj.GnuHashAddr == 0 {
		t.Error("Both obj.HashAddr and obj.GnuHashAddr are zero")
	}
}

func TestLoadInvalidFile(t *testing.T) {
	resolver := &mockResolver{symbols: make(map[string]uintptr)}

	tests := []string{
		"nonexistent.so",
		"/dev/null",
		"loader_test.go",
	}

	for _, path := range tests {
		obj, err := Load(path, resolver)
		if err == nil {
			if obj != nil {
				Unload(obj)
			}
			t.Errorf("Load(%q) should have failed but succeeded", path)
		}
	}
}

func TestUnload(t *testing.T) {
	testLib := "../testdata/libtest.so"
	resolver := &mockResolver{symbols: make(map[string]uintptr)}

	obj, err := Load(testLib, resolver)
	if err != nil {
		t.Fatalf("Load(%q) failed: %v", testLib, err)
	}

	// Unload should succeed (it unmaps memory and runs finalizers)
	err = Unload(obj)
	if err != nil {
		t.Errorf("Unload() failed: %v", err)
	}

	// Note: Unload doesn't modify RefCount - that's handled by the dl package
	// The test just verifies that Unload completes without error
}

func TestObjectFields(t *testing.T) {
	testLib := "../testdata/libtest.so"
	resolver := &mockResolver{symbols: make(map[string]uintptr)}

	obj, err := Load(testLib, resolver)
	if err != nil {
		t.Fatalf("Load(%q) failed: %v", testLib, err)
	}
	defer Unload(obj)

	// Test that Object fields are properly initialized
	if obj.Parsed == nil {
		t.Error("obj.Parsed is nil")
	}
	if obj.Parsed.Path != testLib {
		t.Errorf("obj.Parsed.Path = %q, want %q", obj.Parsed.Path, testLib)
	}

	// Verify segments are mapped correctly
	for i, seg := range obj.Segments {
		if seg.Addr == 0 {
			t.Errorf("Segment[%d].Addr is zero", i)
		}
		if seg.Size == 0 {
			t.Errorf("Segment[%d].Size is zero", i)
		}
		// Protection should be reasonable
		if seg.Prot < 0 {
			t.Errorf("Segment[%d].Prot = %d, invalid", i, seg.Prot)
		}
	}
}

func TestLoadWithSystemLib(t *testing.T) {
	// System library testing requires PURE_GO_DL_TEST_SYSTEM_LIBS=1 due to potential
	// crashes in init/fini functions. See AUDIT finding HIGH-01.
	if os.Getenv("PURE_GO_DL_TEST_SYSTEM_LIBS") != "1" {
		t.Skip("Skipping system library test - set PURE_GO_DL_TEST_SYSTEM_LIBS=1 to enable")
	}

	// Try to load a real system library
	systemLibs := []string{
		"/lib/x86_64-linux-gnu/libm.so.6",
		"/usr/lib/x86_64-linux-gnu/libm.so.6",
		"/lib64/libm.so.6",
	}

	resolver := &mockResolver{symbols: make(map[string]uintptr)}

	for _, lib := range systemLibs {
		obj, err := Load(lib, resolver)
		if err == nil {
			// NOTE: Don't call Unload on system libraries - their fini functions
			// crash because they expect runtime state our minimal loader doesn't provide.
			// This is a known limitation per AUDIT HIGH-01.
			// System libraries can be loaded and used successfully, just not unloaded.

			// Verify basic properties
			if obj.Base == 0 {
				t.Errorf("Load(%q): Base is zero", lib)
			}
			if len(obj.Segments) == 0 {
				t.Errorf("Load(%q): no segments", lib)
			}
			return // Success, don't try other paths
		}
	}

	t.Skip("No system libm.so.6 found, skipping system library load test")
}
