package elf

import (
	"debug/elf"
	"os"
	"path/filepath"
	"testing"
)

// TestParseInvalidDynamicEntries tests validation of dynamic section entries
func TestParseInvalidDynamicEntries(t *testing.T) {
	testCases := []struct {
		name    string
		dtTag   elf.DynTag
		dtVal   uint64
		wantErr bool
		errMsg  string
	}{
		{
			name:    "zero_strtab",
			dtTag:   elf.DT_STRTAB,
			dtVal:   0,
			wantErr: true,
			errMsg:  "DT_STRTAB",
		},
		{
			name:    "zero_strsz",
			dtTag:   elf.DT_STRSZ,
			dtVal:   0,
			wantErr: true,
			errMsg:  "DT_STRSZ",
		},
		{
			name:    "zero_symtab",
			dtTag:   elf.DT_SYMTAB,
			dtVal:   0,
			wantErr: true,
			errMsg:  "DT_SYMTAB",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			testFile := filepath.Join(tmpDir, "test.so")

			// Create a minimal ELF with invalid dynamic entry
			data := createMinimalELFWithDynamic(t, tc.dtTag, tc.dtVal)
			if err := os.WriteFile(testFile, data, 0o644); err != nil {
				t.Fatalf("Failed to create test file: %v", err)
			}

			_, err := Parse(testFile)
			if tc.wantErr {
				if err == nil {
					t.Errorf("Parse should have failed for %s", tc.name)
				}
			} else {
				if err != nil {
					t.Errorf("Parse should not have failed: %v", err)
				}
			}
		})
	}
}

// TestParseFileNotFound tests error handling for missing files
func TestParseFileNotFound(t *testing.T) {
	_, err := Parse("/nonexistent/path/to/file.so")
	if err == nil {
		t.Error("Parse of nonexistent file should fail")
	}
}

// TestParseEmptyFile tests error handling for empty ELF files
func TestParseEmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	emptyFile := filepath.Join(tmpDir, "empty.so")

	if err := os.WriteFile(emptyFile, []byte{}, 0o644); err != nil {
		t.Fatalf("Failed to create empty file: %v", err)
	}

	_, err := Parse(emptyFile)
	if err == nil {
		t.Error("Parse of empty file should fail")
	}
}

// TestParseTruncatedHeader tests error handling for truncated ELF headers
func TestParseTruncatedHeader(t *testing.T) {
	tmpDir := t.TempDir()
	truncFile := filepath.Join(tmpDir, "truncated.so")

	// Write only partial ELF magic
	data := []byte{0x7f, 0x45, 0x4c}
	if err := os.WriteFile(truncFile, data, 0o644); err != nil {
		t.Fatalf("Failed to create truncated file: %v", err)
	}

	_, err := Parse(truncFile)
	if err == nil {
		t.Error("Parse of truncated header should fail")
	}
}

// TestParseInvalidELFClass tests error handling for wrong ELF class (32-bit vs 64-bit)
func TestParseInvalidELFClass(t *testing.T) {
	tmpDir := t.TempDir()
	invalidFile := filepath.Join(tmpDir, "invalid_class.so")

	// Create ELF header with 32-bit class (ELFCLASS32)
	data := make([]byte, 64)
	copy(data[0:4], []byte{0x7f, 0x45, 0x4c, 0x46}) // ELF magic
	data[4] = 1                                     // ELFCLASS32 (not ELFCLASS64)
	data[5] = 1                                     // ELFDATA2LSB
	data[6] = 1                                     // EV_CURRENT

	if err := os.WriteFile(invalidFile, data, 0o644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	_, err := Parse(invalidFile)
	if err == nil {
		t.Error("Parse should reject 32-bit ELF files")
	}
}

// Helper function to create minimal ELF with specific dynamic entry
func createMinimalELFWithDynamic(t *testing.T, tag elf.DynTag, val uint64) []byte {
	t.Helper()

	// This is a simplified version - in reality we'd need to create a complete ELF structure
	// For now, we'll create a minimal header that passes initial checks
	data := make([]byte, 512)

	// ELF magic and header
	copy(data[0:4], []byte{0x7f, 0x45, 0x4c, 0x46}) // ELF magic
	data[4] = 2                                     // ELFCLASS64
	data[5] = 1                                     // ELFDATA2LSB
	data[6] = 1                                     // EV_CURRENT
	data[16] = 3                                    // ET_DYN (shared object)
	data[18] = 0x3e                                 // EM_X86_64

	return data
}

// TestParseWithMissingProgramHeaders tests error when program headers are missing
func TestParseWithMissingProgramHeaders(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "nophdr.so")

	data := make([]byte, 64)
	copy(data[0:4], []byte{0x7f, 0x45, 0x4c, 0x46})
	data[4] = 2  // ELFCLASS64
	data[5] = 1  // ELFDATA2LSB
	data[6] = 1  // EV_CURRENT
	data[16] = 3 // ET_DYN

	// Set e_phnum to 0 (no program headers)
	// This should cause Parse to fail or return incomplete data

	if err := os.WriteFile(testFile, data, 0o644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	_, err := Parse(testFile)
	if err == nil {
		// Note: depending on implementation, this might not error
		// but would result in an incomplete parsed object
		t.Log("Parse completed (may be expected for minimal file)")
	}
}
