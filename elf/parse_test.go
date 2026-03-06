package elf

import (
	"debug/elf"
	"testing"
)

func TestPageAlign(t *testing.T) {
	tests := []struct {
		input uint64
		want  uint64
	}{
		{0, 0},
		{1, 4096},
		{4095, 4096},
		{4096, 4096},
		{4097, 8192},
		{8192, 8192},
		{16384, 16384},
	}
	for _, tt := range tests {
		got := PageAlign(tt.input)
		if got != tt.want {
			t.Errorf("PageAlign(%d) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestReadCString(t *testing.T) {
	data := []byte{'h', 'e', 'l', 'l', 'o', 0, 'w', 'o', 'r', 'l', 'd', 0}
	tests := []struct {
		off  int
		want string
	}{
		{0, "hello"},
		{6, "world"},
		{11, ""},
		{12, ""},
		{-1, ""},
		{100, ""},
	}
	for _, tt := range tests {
		got := readCString(data, tt.off)
		if got != tt.want {
			t.Errorf("readCString(data, %d) = %q, want %q", tt.off, got, tt.want)
		}
	}
}

func TestParse(t *testing.T) {
	// Test with actual test library
	testLib := "../testdata/libtest.so"
	obj, err := Parse(testLib)
	if err != nil {
		t.Fatalf("Parse(%q) failed: %v", testLib, err)
	}

	// Verify basic structure
	if obj.Path != testLib {
		t.Errorf("obj.Path = %q, want %q", obj.Path, testLib)
	}
	if obj.File == nil {
		t.Fatal("obj.File is nil")
	}
	if len(obj.LoadSegments) == 0 {
		t.Error("obj.LoadSegments is empty")
	}
	if obj.DynamicSeg == nil {
		t.Error("obj.DynamicSeg is nil")
	}

	// Verify address space calculation
	if obj.BaseVAddr == 0 && obj.MemSize == 0 {
		t.Error("BaseVAddr and MemSize are both zero")
	}
	if obj.MemSize%4096 != 0 {
		t.Errorf("obj.MemSize = %d, not page-aligned", obj.MemSize)
	}

	// Verify dynamic entries were parsed
	if len(obj.DynEntries) == 0 {
		t.Error("DynEntries is empty")
	}

	// Check for expected dynamic tags
	expectedTags := []elf.DynTag{
		elf.DT_STRTAB,
		elf.DT_SYMTAB,
		elf.DT_HASH, // or DT_GNU_HASH
	}
	for _, tag := range expectedTags {
		if _, ok := obj.DynEntries[tag]; !ok {
			if tag == elf.DT_HASH {
				// Check for GNU_HASH as alternative
				if _, ok := obj.DynEntries[elf.DT_GNU_HASH]; !ok {
					t.Errorf("DynEntries missing both DT_HASH and DT_GNU_HASH")
				}
			} else {
				t.Errorf("DynEntries missing %v", tag)
			}
		}
	}

	// Verify DT_NEEDED dependencies (libtest.so should depend on libc)
	if len(obj.Needed) == 0 {
		t.Log("Warning: no DT_NEEDED entries found (expected at least libc.so.6)")
	}
}

func TestParseWithSystemLib(t *testing.T) {
	// Test with system library if available
	systemLibs := []string{
		"/lib/x86_64-linux-gnu/libm.so.6",
		"/usr/lib/x86_64-linux-gnu/libm.so.6",
		"/lib64/libm.so.6",
	}

	var testLib string
	for _, lib := range systemLibs {
		obj, err := Parse(lib)
		if err == nil {
			testLib = lib
			if obj == nil {
				t.Fatalf("Parse(%q) returned nil object", lib)
			}
			if obj.File == nil {
				t.Fatalf("Parse(%q) returned nil File", lib)
			}
			if len(obj.LoadSegments) == 0 {
				t.Errorf("Parse(%q) has no load segments", lib)
			}
			break
		}
	}
	if testLib == "" {
		t.Skip("No system libm.so.6 found, skipping system library test")
	}
}

func TestParseInvalidFile(t *testing.T) {
	tests := []struct {
		path    string
		wantErr bool
	}{
		{"nonexistent.so", true},
		{"/dev/null", true},
		{"parse_test.go", true}, // not an ELF file
	}

	for _, tt := range tests {
		_, err := Parse(tt.path)
		if (err != nil) != tt.wantErr {
			t.Errorf("Parse(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
		}
	}
}

func TestReadBytesAtVAddr(t *testing.T) {
	// This function is tested indirectly by TestParse which calls it internally
	// If Parse succeeds, we know readBytesAtVAddr works correctly
	testLib := "../testdata/libtest.so"
	obj, err := Parse(testLib)
	if err != nil {
		t.Fatalf("Parse(%q) failed: %v", testLib, err)
	}

	// Verify that the string table was read (which uses readBytesAtVAddr)
	if _, ok := obj.DynEntries[elf.DT_STRTAB]; !ok {
		t.Skip("No DT_STRTAB found, cannot verify readBytesAtVAddr")
	}
	
	// If we have DT_NEEDED entries, readBytesAtVAddr worked
	if len(obj.Needed) > 0 {
		t.Logf("Successfully read %d DT_NEEDED entries using readBytesAtVAddr", len(obj.Needed))
	}
}

func TestParsedObjectFields(t *testing.T) {
	testLib := "../testdata/libtest.so"
	obj, err := Parse(testLib)
	if err != nil {
		t.Fatalf("Parse(%q) failed: %v", testLib, err)
	}

	// Test that all expected fields are populated
	if obj.Path == "" {
		t.Error("Path is empty")
	}
	if obj.File == nil {
		t.Error("File is nil")
	}
	if obj.BaseVAddr > obj.MemSize && obj.MemSize != 0 {
		t.Errorf("BaseVAddr (%d) > MemSize (%d)", obj.BaseVAddr, obj.MemSize)
	}
	if len(obj.DynData) == 0 {
		t.Error("DynData is empty")
	}
	if obj.DynVAddr == 0 {
		t.Error("DynVAddr is zero")
	}
}

func TestParseTLSSegment(t *testing.T) {
	// libtest.so might not have TLS, but we can test that TLS detection works
	testLib := "../testdata/libtest.so"
	obj, err := Parse(testLib)
	if err != nil {
		t.Fatalf("Parse(%q) failed: %v", testLib, err)
	}

	// TLS segment is optional
	if obj.TLSSeg != nil {
		if obj.TLSSeg.Type != elf.PT_TLS {
			t.Errorf("TLSSeg type = %v, want PT_TLS", obj.TLSSeg.Type)
		}
	}
}

func TestParseGNURelro(t *testing.T) {
	testLib := "../testdata/libtest.so"
	obj, err := Parse(testLib)
	if err != nil {
		t.Fatalf("Parse(%q) failed: %v", testLib, err)
	}

	// GNU_RELRO is optional but commonly present
	if obj.GNURelroSeg != nil {
		if obj.GNURelroSeg.Type != elf.PT_GNU_RELRO {
			t.Errorf("GNURelroSeg type = %v, want PT_GNU_RELRO", obj.GNURelroSeg.Type)
		}
	}
}
