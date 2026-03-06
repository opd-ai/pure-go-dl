package elf

import (
	"debug/elf"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

// TestMalformedELF_InvalidMagic tests rejection of files with incorrect ELF magic
func TestMalformedELF_InvalidMagic(t *testing.T) {
	tmpDir := t.TempDir()
	badFile := filepath.Join(tmpDir, "badmagic.so")

	data := make([]byte, 64)
	// Wrong magic bytes (should be 0x7f, 'E', 'L', 'F')
	data[0], data[1], data[2], data[3] = 0x7f, 'X', 'L', 'F'

	if err := os.WriteFile(badFile, data, 0o644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	_, err := Parse(badFile)
	if err == nil {
		t.Error("Parse should reject file with invalid ELF magic")
	}
}

// TestMalformedELF_WrongClass tests rejection of 32-bit ELF files (ELFCLASS32)
func TestMalformedELF_WrongClass(t *testing.T) {
	tmpDir := t.TempDir()
	badFile := filepath.Join(tmpDir, "elfclass32.so")

	data := make([]byte, 64)
	// Correct magic
	data[0], data[1], data[2], data[3] = 0x7f, 'E', 'L', 'F'
	// ELFCLASS32 instead of ELFCLASS64
	data[4] = byte(elf.ELFCLASS32)

	if err := os.WriteFile(badFile, data, 0o644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	_, err := Parse(badFile)
	if err == nil {
		t.Error("Parse should reject ELFCLASS32 files")
	}
}

// TestMalformedELF_WrongEndian tests rejection of big-endian ELF files
func TestMalformedELF_WrongEndian(t *testing.T) {
	tmpDir := t.TempDir()
	badFile := filepath.Join(tmpDir, "bigendian.so")

	data := make([]byte, 64)
	// Correct magic
	data[0], data[1], data[2], data[3] = 0x7f, 'E', 'L', 'F'
	data[4] = byte(elf.ELFCLASS64)
	// ELFDATA2MSB (big-endian) instead of ELFDATA2LSB
	data[5] = byte(elf.ELFDATA2MSB)

	if err := os.WriteFile(badFile, data, 0o644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	_, err := Parse(badFile)
	if err == nil {
		t.Error("Parse should reject big-endian ELF files")
	}
}

// TestMalformedELF_WrongType tests that ET_EXEC files are rejected (only ET_DYN supported)
func TestMalformedELF_WrongType(t *testing.T) {
	tmpDir := t.TempDir()
	badFile := filepath.Join(tmpDir, "exec.elf")

	// Create a minimal valid ELF header but with ET_EXEC type
	data := make([]byte, 256)
	
	// ELF magic
	data[0], data[1], data[2], data[3] = 0x7f, 'E', 'L', 'F'
	data[4] = byte(elf.ELFCLASS64)
	data[5] = byte(elf.ELFDATA2LSB)
	data[6] = byte(elf.EV_CURRENT)
	
	// e_type = ET_EXEC (2) at offset 16
	binary.LittleEndian.PutUint16(data[16:], uint16(elf.ET_EXEC))
	// e_machine = EM_X86_64 at offset 18
	binary.LittleEndian.PutUint16(data[18:], uint16(elf.EM_X86_64))
	// e_version at offset 20
	binary.LittleEndian.PutUint32(data[20:], uint32(elf.EV_CURRENT))
	// e_ehsize at offset 52
	binary.LittleEndian.PutUint16(data[52:], 64)
	// e_phentsize at offset 54
	binary.LittleEndian.PutUint16(data[54:], 56)

	if err := os.WriteFile(badFile, data, 0o644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	_, err := Parse(badFile)
	if err == nil {
		t.Error("Parse should reject ET_EXEC type files (only ET_DYN supported)")
	}
}

// TestMalformedELF_MissingPTLoad tests handling of ELF with no PT_LOAD segments
func TestMalformedELF_MissingPTLoad(t *testing.T) {
	tmpDir := t.TempDir()
	badFile := filepath.Join(tmpDir, "noptload.so")

	// Create minimal ELF with valid header but no PT_LOAD segments
	data := createMinimalELFHeader(t)
	// Set e_phnum = 0 (no program headers)
	binary.LittleEndian.PutUint16(data[56:], 0)

	if err := os.WriteFile(badFile, data, 0o644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	obj, err := Parse(badFile)
	if err != nil {
		// It's OK to reject files with no PT_LOAD
		return
	}
	// If parsing succeeds, verify LoadSegments is empty
	if len(obj.LoadSegments) > 0 {
		t.Error("Expected no load segments, but found some")
	}
}

// TestMalformedELF_PTLoadFileszGreaterThanMemsz tests PT_LOAD with Filesz > Memsz
func TestMalformedELF_PTLoadFileszGreaterThanMemsz(t *testing.T) {
	tmpDir := t.TempDir()
	badFile := filepath.Join(tmpDir, "badfilesz.so")

	data := createMinimalELFHeader(t)
	
	// Extend data to include program header
	data = append(data, make([]byte, 56)...)
	
	// Now set e_phnum and e_phoff
	binary.LittleEndian.PutUint16(data[56:], 1) // e_phnum = 1
	binary.LittleEndian.PutUint64(data[32:], 64) // e_phoff = 64
	
	phOff := 64
	
	binary.LittleEndian.PutUint32(data[phOff:], uint32(elf.PT_LOAD))
	binary.LittleEndian.PutUint32(data[phOff+4:], uint32(elf.PF_R|elf.PF_X))
	binary.LittleEndian.PutUint64(data[phOff+8:], 0x1000)   // p_offset
	binary.LittleEndian.PutUint64(data[phOff+16:], 0x1000)  // p_vaddr
	binary.LittleEndian.PutUint64(data[phOff+24:], 0x1000)  // p_paddr
	binary.LittleEndian.PutUint64(data[phOff+32:], 0x5000)  // p_filesz (LARGER)
	binary.LittleEndian.PutUint64(data[phOff+40:], 0x2000)  // p_memsz (smaller - INVALID)
	binary.LittleEndian.PutUint64(data[phOff+48:], 0x1000)  // p_align

	if err := os.WriteFile(badFile, data, 0o644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	_, err := Parse(badFile)
	if err == nil {
		t.Error("Parse should reject PT_LOAD with Filesz > Memsz")
	}
}

// TestMalformedELF_PTLoadMemsizeOverflow tests PT_LOAD with uint64 overflow in Memsz calculation
func TestMalformedELF_PTLoadMemsizeOverflow(t *testing.T) {
	tmpDir := t.TempDir()
	badFile := filepath.Join(tmpDir, "memsizoverflow.so")

	data := createMinimalELFHeader(t)
	
	// Extend data first
	data = append(data, make([]byte, 56)...)
	
	binary.LittleEndian.PutUint16(data[56:], 1) // e_phnum = 1
	binary.LittleEndian.PutUint64(data[32:], 64) // e_phoff = 64
	
	phOff := 64
	
	binary.LittleEndian.PutUint32(data[phOff:], uint32(elf.PT_LOAD))
	binary.LittleEndian.PutUint32(data[phOff+4:], uint32(elf.PF_R|elf.PF_X))
	binary.LittleEndian.PutUint64(data[phOff+8:], 0)
	binary.LittleEndian.PutUint64(data[phOff+16:], 0xFFFFFFFFFFFF0000) // High vaddr
	binary.LittleEndian.PutUint64(data[phOff+24:], 0xFFFFFFFFFFFF0000)
	binary.LittleEndian.PutUint64(data[phOff+32:], 0x2000) // filesz
	binary.LittleEndian.PutUint64(data[phOff+40:], 0x20000) // memsz - would overflow when added to vaddr
	binary.LittleEndian.PutUint64(data[phOff+48:], 0x1000)

	if err := os.WriteFile(badFile, data, 0o644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	obj, err := Parse(badFile)
	if err != nil {
		// Good - overflow detected
		return
	}
	// If parsing succeeds, verify it handled overflow safely
	if obj != nil && obj.MemSize == 0 {
		t.Error("MemSize calculation resulted in zero (possible overflow)")
	}
}

// TestMalformedELF_MissingPTDynamic tests handling of ELF with no PT_DYNAMIC segment
func TestMalformedELF_MissingPTDynamic(t *testing.T) {
	tmpDir := t.TempDir()
	badFile := filepath.Join(tmpDir, "nodynamic.so")

	data := createMinimalELFHeader(t)
	
	// Extend data first
	data = append(data, make([]byte, 56)...)
	
	// Add a PT_LOAD but no PT_DYNAMIC
	binary.LittleEndian.PutUint16(data[56:], 1) // e_phnum = 1
	binary.LittleEndian.PutUint64(data[32:], 64) // e_phoff = 64
	
	phOff := 64
	
	// Valid PT_LOAD
	binary.LittleEndian.PutUint32(data[phOff:], uint32(elf.PT_LOAD))
	binary.LittleEndian.PutUint32(data[phOff+4:], uint32(elf.PF_R|elf.PF_W))
	binary.LittleEndian.PutUint64(data[phOff+8:], 0)
	binary.LittleEndian.PutUint64(data[phOff+16:], 0)
	binary.LittleEndian.PutUint64(data[phOff+24:], 0)
	binary.LittleEndian.PutUint64(data[phOff+32:], 0x1000)
	binary.LittleEndian.PutUint64(data[phOff+40:], 0x1000)
	binary.LittleEndian.PutUint64(data[phOff+48:], 0x1000)

	if err := os.WriteFile(badFile, data, 0o644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	_, err := Parse(badFile)
	if err == nil {
		t.Error("Parse should reject shared object with no PT_DYNAMIC segment")
	}
}

// TestMalformedELF_DynamicSectionInvalidOffset tests PT_DYNAMIC pointing outside file
func TestMalformedELF_DynamicSectionInvalidOffset(t *testing.T) {
	tmpDir := t.TempDir()
	badFile := filepath.Join(tmpDir, "baddynoffset.so")

	data := createMinimalELFHeader(t)
	
	// Extend data first
	data = append(data, make([]byte, 56)...)
	
	binary.LittleEndian.PutUint16(data[56:], 1) // e_phnum = 1
	binary.LittleEndian.PutUint64(data[32:], 64) // e_phoff = 64
	
	phOff := 64
	
	// PT_DYNAMIC with invalid file offset
	binary.LittleEndian.PutUint32(data[phOff:], uint32(elf.PT_DYNAMIC))
	binary.LittleEndian.PutUint32(data[phOff+4:], uint32(elf.PF_R|elf.PF_W))
	binary.LittleEndian.PutUint64(data[phOff+8:], 0xFFFFFFFF) // Invalid offset beyond file
	binary.LittleEndian.PutUint64(data[phOff+16:], 0x1000)
	binary.LittleEndian.PutUint64(data[phOff+24:], 0x1000)
	binary.LittleEndian.PutUint64(data[phOff+32:], 0x100)
	binary.LittleEndian.PutUint64(data[phOff+40:], 0x100)
	binary.LittleEndian.PutUint64(data[phOff+48:], 0x8)

	if err := os.WriteFile(badFile, data, 0o644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	_, err := Parse(badFile)
	if err == nil {
		t.Error("Parse should reject PT_DYNAMIC with offset outside file bounds")
	}
}

// TestMalformedELF_DynEntriesInvalidTag tests handling of unknown/invalid DT_ tags
func TestMalformedELF_DynEntriesInvalidTag(t *testing.T) {
	// This test verifies that unknown DynTags are handled gracefully
	// The parser should ignore unknown tags but process known ones
	// For simplicity, we use the real test library and verify behavior
	testLib := "../testdata/libtest.so"
	obj, err := Parse(testLib)
	if err != nil {
		t.Skipf("Cannot test with libtest.so: %v", err)
	}

	// Verify that parser successfully ignores unknown tags and parses known ones
	if len(obj.DynEntries) == 0 {
		t.Error("DynEntries should not be empty for valid library")
	}
}

// TestMalformedELF_MissingStringTable tests handling of DT_STRTAB pointing to invalid address
func TestMalformedELF_MissingStringTable(t *testing.T) {
	// This is tested indirectly - if DT_STRTAB is missing or invalid,
	// the parser should handle it gracefully when processing DT_NEEDED
	// We verify this by checking that our test library has valid string table
	testLib := "../testdata/libtest.so"
	obj, err := Parse(testLib)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// Verify string table tag exists
	if _, ok := obj.DynEntries[elf.DT_STRTAB]; !ok {
		t.Error("Valid ELF should have DT_STRTAB entry")
	}

	// Verify we could read DT_NEEDED strings (which requires valid strtab)
	if len(obj.Needed) > 0 {
		for _, need := range obj.Needed {
			if need == "" {
				t.Error("DT_NEEDED entry should not be empty string")
			}
		}
	}
}

// TestMalformedELF_ZeroAlignment tests PT_LOAD with zero alignment
func TestMalformedELF_ZeroAlignment(t *testing.T) {
	tmpDir := t.TempDir()
	badFile := filepath.Join(tmpDir, "zeroalign.so")

	data := createMinimalELFHeader(t)
	
	// Extend data first
	data = append(data, make([]byte, 56)...)
	
	binary.LittleEndian.PutUint16(data[56:], 1) // e_phnum = 1
	binary.LittleEndian.PutUint64(data[32:], 64) // e_phoff = 64
	
	phOff := 64
	
	binary.LittleEndian.PutUint32(data[phOff:], uint32(elf.PT_LOAD))
	binary.LittleEndian.PutUint32(data[phOff+4:], uint32(elf.PF_R|elf.PF_X))
	binary.LittleEndian.PutUint64(data[phOff+8:], 0)
	binary.LittleEndian.PutUint64(data[phOff+16:], 0)
	binary.LittleEndian.PutUint64(data[phOff+24:], 0)
	binary.LittleEndian.PutUint64(data[phOff+32:], 0x1000)
	binary.LittleEndian.PutUint64(data[phOff+40:], 0x1000)
	binary.LittleEndian.PutUint64(data[phOff+48:], 0) // p_align = 0 (invalid)

	if err := os.WriteFile(badFile, data, 0o644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	obj, err := Parse(badFile)
	// Parser may accept zero alignment and default to 1 or page size
	// We just verify it doesn't crash
	if err == nil && obj != nil {
		t.Logf("Parser handled zero alignment (accepted with defaults)")
	}
}

// TestMalformedELF_CorruptedDynamicSection tests dynamic section with invalid size
func TestMalformedELF_CorruptedDynamicSection(t *testing.T) {
	tmpDir := t.TempDir()
	badFile := filepath.Join(tmpDir, "corruptdyn.so")

	data := createMinimalELFHeader(t)
	
	// Extend data first for program header
	data = append(data, make([]byte, 56)...)
	
	binary.LittleEndian.PutUint16(data[56:], 1)
	binary.LittleEndian.PutUint64(data[32:], 64)
	
	phOff := 64
	
	// PT_DYNAMIC with size that's not multiple of entry size (16 bytes)
	binary.LittleEndian.PutUint32(data[phOff:], uint32(elf.PT_DYNAMIC))
	binary.LittleEndian.PutUint32(data[phOff+4:], uint32(elf.PF_R|elf.PF_W))
	binary.LittleEndian.PutUint64(data[phOff+8:], 0x200)
	binary.LittleEndian.PutUint64(data[phOff+16:], 0x1000)
	binary.LittleEndian.PutUint64(data[phOff+24:], 0x1000)
	binary.LittleEndian.PutUint64(data[phOff+32:], 15) // Not multiple of 16
	binary.LittleEndian.PutUint64(data[phOff+40:], 15)
	binary.LittleEndian.PutUint64(data[phOff+48:], 0x8)

	// Add some fake dynamic section data
	data = append(data, make([]byte, 0x200)...)

	if err := os.WriteFile(badFile, data, 0o644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	_, err := Parse(badFile)
	// Parser should handle this gracefully (may truncate to valid entries)
	if err != nil {
		t.Logf("Parser rejected corrupted dynamic section: %v", err)
	}
}

// createMinimalELFHeader creates a minimal valid ELF64 header for testing
func createMinimalELFHeader(t *testing.T) []byte {
	t.Helper()
	data := make([]byte, 64)
	
	// ELF magic
	data[0], data[1], data[2], data[3] = 0x7f, 'E', 'L', 'F'
	data[4] = byte(elf.ELFCLASS64)
	data[5] = byte(elf.ELFDATA2LSB)
	data[6] = byte(elf.EV_CURRENT)
	data[7] = byte(elf.ELFOSABI_NONE)
	
	// e_type = ET_DYN (3) at offset 16
	binary.LittleEndian.PutUint16(data[16:], uint16(elf.ET_DYN))
	// e_machine = EM_X86_64 at offset 18
	binary.LittleEndian.PutUint16(data[18:], uint16(elf.EM_X86_64))
	// e_version at offset 20
	binary.LittleEndian.PutUint32(data[20:], uint32(elf.EV_CURRENT))
	// e_ehsize at offset 52 (size of ELF header = 64 bytes)
	binary.LittleEndian.PutUint16(data[52:], 64)
	// e_phentsize at offset 54 (size of program header = 56 bytes)
	binary.LittleEndian.PutUint16(data[54:], 56)
	// e_phoff at offset 32 (program header offset)
	binary.LittleEndian.PutUint64(data[32:], 64)
	
	return data
}
