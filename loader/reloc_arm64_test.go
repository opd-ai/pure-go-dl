//go:build arm64 && linux

package loader

import (
	"testing"
)

// TestARM64RelocConstants verifies that ARM64-specific relocation constants are properly defined.
func TestARM64RelocConstants(t *testing.T) {
	// Verify critical relocation type constants
	tests := []struct {
		name  string
		value uint32
		want  uint32
	}{
		{"R_AARCH64_NONE", R_AARCH64_NONE, 0},
		{"R_AARCH64_ABS64", R_AARCH64_ABS64, 257},
		{"R_AARCH64_ABS32", R_AARCH64_ABS32, 258},
		{"R_AARCH64_RELATIVE", R_AARCH64_RELATIVE, 1027},
		{"R_AARCH64_GLOB_DAT", R_AARCH64_GLOB_DAT, 1025},
		{"R_AARCH64_JUMP_SLOT", R_AARCH64_JUMP_SLOT, 1026},
		{"R_AARCH64_COPY", R_AARCH64_COPY, 1024},
		{"R_AARCH64_TLS_DTPMOD64", R_AARCH64_TLS_DTPMOD64, 1028},
		{"R_AARCH64_TLS_DTPREL64", R_AARCH64_TLS_DTPREL64, 1029},
		{"R_AARCH64_TLS_TPREL64", R_AARCH64_TLS_TPREL64, 1030},
		{"R_AARCH64_IRELATIVE", R_AARCH64_IRELATIVE, 1032},
	}

	for _, tt := range tests {
		if tt.value != tt.want {
			t.Errorf("%s = %d, want %d", tt.name, tt.value, tt.want)
		}
	}
}

// TestARM64ArchMappings verifies that architecture-independent constant mappings are correct.
func TestARM64ArchMappings(t *testing.T) {
	tests := []struct {
		name  string
		value uint32
		want  uint32
	}{
		{"relocNone", relocNone, R_AARCH64_NONE},
		{"relocRelative", relocRelative, R_AARCH64_RELATIVE},
		{"reloc64", reloc64, R_AARCH64_ABS64},
		{"relocGlobDat", relocGlobDat, R_AARCH64_GLOB_DAT},
		{"relocJumpSlot", relocJumpSlot, R_AARCH64_JUMP_SLOT},
		{"relocCopy", relocCopy, R_AARCH64_COPY},
		{"reloc32", reloc32, R_AARCH64_ABS32},
		{"relocDTPMod64", relocDTPMod64, R_AARCH64_TLS_DTPMOD64},
		{"relocDTPOff64", relocDTPOff64, R_AARCH64_TLS_DTPREL64},
		{"relocTPOff64", relocTPOff64, R_AARCH64_TLS_TPREL64},
		{"relocIRelative", relocIRelative, R_AARCH64_IRELATIVE},
	}

	for _, tt := range tests {
		if tt.value != tt.want {
			t.Errorf("%s = %d, want %d", tt.name, tt.value, tt.want)
		}
	}
}

// TestARM64RelaFunctions verifies that RELA info extraction functions work correctly.
func TestARM64RelaFunctions(t *testing.T) {
	// Test symbol index extraction (upper 32 bits)
	info := uint64(0x0000000500000001) // sym=5, type=1
	symIdx := relaSymIdx(info)
	if symIdx != 5 {
		t.Errorf("relaSymIdx(0x%x) = %d, want 5", info, symIdx)
	}

	// Test relocation type extraction (lower 32 bits)
	relType := relaType(info)
	if relType != 1 {
		t.Errorf("relaType(0x%x) = %d, want 1", info, relType)
	}

	// Test with larger values
	info = uint64(0x0000FFFF00000400) // sym=65535, type=1024 (R_AARCH64_COPY)
	symIdx = relaSymIdx(info)
	if symIdx != 65535 {
		t.Errorf("relaSymIdx(0x%x) = %d, want 65535", info, symIdx)
	}

	relType = relaType(info)
	if relType != 1024 {
		t.Errorf("relaType(0x%x) = %d, want 1024", info, relType)
	}
}

// TestARM64RelaEntSize verifies the RELA entry size constant.
func TestARM64RelaEntSize(t *testing.T) {
	// ARM64 uses 64-bit ELF format, so Elf64_Rela is 24 bytes:
	// - r_offset: 8 bytes
	// - r_info:   8 bytes
	// - r_addend: 8 bytes
	if relaEntSize != 24 {
		t.Errorf("relaEntSize = %d, want 24", relaEntSize)
	}
}

// TestARM64TLSRelocations verifies TLS relocation constants are correct.
func TestARM64TLSRelocations(t *testing.T) {
	tests := []struct {
		name  string
		value uint32
		want  uint32
	}{
		// Thread-Local Storage relocations
		{"R_AARCH64_TLSGD_ADR_PAGE21", R_AARCH64_TLSGD_ADR_PAGE21, 513},
		{"R_AARCH64_TLSLD_ADR_PAGE21", R_AARCH64_TLSLD_ADR_PAGE21, 518},
		{"R_AARCH64_TLSIE_LD64_GOTTPREL_LO12_NC", R_AARCH64_TLSIE_LD64_GOTTPREL_LO12_NC, 542},
		{"R_AARCH64_TLSLE_ADD_TPREL_HI12", R_AARCH64_TLSLE_ADD_TPREL_HI12, 549},
		{"R_AARCH64_TLSDESC", R_AARCH64_TLSDESC, 1031},
	}

	for _, tt := range tests {
		if tt.value != tt.want {
			t.Errorf("%s = %d, want %d", tt.name, tt.value, tt.want)
		}
	}
}

// TestARM64GOTRelocations verifies GOT-related relocation constants.
func TestARM64GOTRelocations(t *testing.T) {
	tests := []struct {
		name  string
		value uint32
		want  uint32
	}{
		{"R_AARCH64_ADR_GOT_PAGE", R_AARCH64_ADR_GOT_PAGE, 311},
		{"R_AARCH64_LD64_GOT_LO12_NC", R_AARCH64_LD64_GOT_LO12_NC, 312},
		{"R_AARCH64_LD64_GOTPAGE_LO15", R_AARCH64_LD64_GOTPAGE_LO15, 313},
	}

	for _, tt := range tests {
		if tt.value != tt.want {
			t.Errorf("%s = %d, want %d", tt.name, tt.value, tt.want)
		}
	}
}

// TestARM64BranchRelocations verifies branch/call relocation constants.
func TestARM64BranchRelocations(t *testing.T) {
	tests := []struct {
		name  string
		value uint32
		want  uint32
	}{
		{"R_AARCH64_JUMP26", R_AARCH64_JUMP26, 282},
		{"R_AARCH64_CALL26", R_AARCH64_CALL26, 283},
		{"R_AARCH64_CONDBR19", R_AARCH64_CONDBR19, 280},
	}

	for _, tt := range tests {
		if tt.value != tt.want {
			t.Errorf("%s = %d, want %d", tt.name, tt.value, tt.want)
		}
	}
}

// TestARM64DataRelocations verifies data access relocation constants.
func TestARM64DataRelocations(t *testing.T) {
	tests := []struct {
		name  string
		value uint32
		want  uint32
	}{
		// Absolute relocations
		{"R_AARCH64_ABS16", R_AARCH64_ABS16, 259},
		{"R_AARCH64_PREL64", R_AARCH64_PREL64, 260},
		{"R_AARCH64_PREL32", R_AARCH64_PREL32, 261},
		{"R_AARCH64_PREL16", R_AARCH64_PREL16, 262},

		// ADRP/ADD instruction pair relocations
		{"R_AARCH64_ADR_PREL_LO21", R_AARCH64_ADR_PREL_LO21, 274},
		{"R_AARCH64_ADR_PREL_PG_HI21", R_AARCH64_ADR_PREL_PG_HI21, 275},
		{"R_AARCH64_ADD_ABS_LO12_NC", R_AARCH64_ADD_ABS_LO12_NC, 277},

		// Load/store relocations
		{"R_AARCH64_LDST8_ABS_LO12_NC", R_AARCH64_LDST8_ABS_LO12_NC, 278},
		{"R_AARCH64_LDST16_ABS_LO12_NC", R_AARCH64_LDST16_ABS_LO12_NC, 284},
		{"R_AARCH64_LDST32_ABS_LO12_NC", R_AARCH64_LDST32_ABS_LO12_NC, 285},
		{"R_AARCH64_LDST64_ABS_LO12_NC", R_AARCH64_LDST64_ABS_LO12_NC, 286},
		{"R_AARCH64_LDST128_ABS_LO12_NC", R_AARCH64_LDST128_ABS_LO12_NC, 299},
	}

	for _, tt := range tests {
		if tt.value != tt.want {
			t.Errorf("%s = %d, want %d", tt.name, tt.value, tt.want)
		}
	}
}

// TestARM64MOVWRelocations verifies MOVW instruction relocations.
func TestARM64MOVWRelocations(t *testing.T) {
	tests := []struct {
		name  string
		value uint32
		want  uint32
	}{
		{"R_AARCH64_MOVW_UABS_G0", R_AARCH64_MOVW_UABS_G0, 263},
		{"R_AARCH64_MOVW_UABS_G0_NC", R_AARCH64_MOVW_UABS_G0_NC, 264},
		{"R_AARCH64_MOVW_UABS_G1", R_AARCH64_MOVW_UABS_G1, 265},
		{"R_AARCH64_MOVW_UABS_G1_NC", R_AARCH64_MOVW_UABS_G1_NC, 266},
		{"R_AARCH64_MOVW_UABS_G2", R_AARCH64_MOVW_UABS_G2, 267},
		{"R_AARCH64_MOVW_UABS_G2_NC", R_AARCH64_MOVW_UABS_G2_NC, 268},
		{"R_AARCH64_MOVW_UABS_G3", R_AARCH64_MOVW_UABS_G3, 269},
	}

	for _, tt := range tests {
		if tt.value != tt.want {
			t.Errorf("%s = %d, want %d", tt.name, tt.value, tt.want)
		}
	}
}
