package symbol

import (
	"debug/elf"
	"encoding/binary"
	"fmt"
	"unsafe"
)

// VersionRequirement describes a required version for a symbol dependency.
type VersionRequirement struct {
	Name  string
	Index uint16
}

// VersionDefinition describes a version definition provided by this library.
type VersionDefinition struct {
	Name  string
	Index uint16
}

// VersionTable holds parsed symbol version information.
type VersionTable struct {
	// Requirements: versions this library needs from its dependencies
	Requirements map[uint16]*VersionRequirement
	
	// Definitions: versions this library provides (export side)
	Definitions map[uint16]*VersionDefinition
	
	// SymbolVersions: maps symbol index → version index (from DT_VERSYM)
	SymbolVersions []uint16
}

// NewVersionTable creates an empty VersionTable.
func NewVersionTable() *VersionTable {
	return &VersionTable{
		Requirements:   make(map[uint16]*VersionRequirement),
		Definitions:    make(map[uint16]*VersionDefinition),
		SymbolVersions: nil,
	}
}

// ParseVersionTables parses DT_VERSYM, DT_VERNEED, and DT_VERDEF from mapped memory.
//
// Parameters:
//   - versymAddr: address of DT_VERSYM table (array of uint16, one per symbol)
//   - versymCount: number of entries in versym table (same as symbol count)
//   - verneedAddr: address of DT_VERNEED structures
//   - verneedNum: DT_VERNEEDNUM (number of Verneed entries)
//   - verdefAddr: address of DT_VERDEF structures
//   - verdefNum: DT_VERDEFNUM (number of Verdef entries)
//   - strtabAddr: address of DT_STRTAB (for version name strings)
func (vt *VersionTable) ParseVersionTables(
	versymAddr uintptr, versymCount uint64,
	verneedAddr uintptr, verneedNum uint64,
	verdefAddr uintptr, verdefNum uint64,
	strtabAddr uintptr,
) error {
	// Parse DT_VERSYM: array of uint16 version indices, one per symbol.
	if versymAddr != 0 && versymCount > 0 {
		vt.SymbolVersions = unsafe.Slice((*uint16)(unsafe.Pointer(versymAddr)), versymCount)
	}

	// Parse DT_VERNEED: version requirements (dependencies).
	if verneedAddr != 0 && verneedNum > 0 {
		if err := vt.parseVerneed(verneedAddr, verneedNum, strtabAddr); err != nil {
			return fmt.Errorf("parse verneed: %w", err)
		}
	}

	// Parse DT_VERDEF: version definitions (exports).
	if verdefAddr != 0 && verdefNum > 0 {
		if err := vt.parseVerdef(verdefAddr, verdefNum, strtabAddr); err != nil {
			return fmt.Errorf("parse verdef: %w", err)
		}
	}

	return nil
}

// parseVerneed reads the Verneed chain.
// Each Verneed entry describes versions required from one dependency library.
// Each Verneed has a linked list of Vernaux entries describing specific versions.
//
// Verneed structure:
//   uint16 vn_version  (version of structure, always 1)
//   uint16 vn_cnt      (number of Vernaux entries)
//   uint32 vn_file     (string table offset for library name)
//   uint32 vn_aux      (offset to first Vernaux)
//   uint32 vn_next     (offset to next Verneed, 0 if last)
//
// Vernaux structure:
//   uint32 vna_hash    (hash of version name)
//   uint16 vna_flags   (flags)
//   uint16 vna_other   (version index)
//   uint32 vna_name    (string table offset)
//   uint32 vna_next    (offset to next Vernaux, 0 if last)
func (vt *VersionTable) parseVerneed(addr uintptr, count uint64, strtabAddr uintptr) error {
	current := addr
	for i := uint64(0); i < count; i++ {
		// Read Verneed header (16 bytes).
		vnVersion := *(*uint16)(unsafe.Pointer(current))
		vnCnt := *(*uint16)(unsafe.Pointer(current + 2))
		_ = *(*uint32)(unsafe.Pointer(current + 4))  // vn_file (library name, not used for index)
		vnAux := *(*uint32)(unsafe.Pointer(current + 8))
		vnNext := *(*uint32)(unsafe.Pointer(current + 12))

		if vnVersion != 1 {
			return fmt.Errorf("unsupported verneed version %d", vnVersion)
		}

		// Walk the Vernaux chain.
		auxCurrent := current + uintptr(vnAux)
		for j := uint16(0); j < vnCnt; j++ {
			// Read Vernaux (20 bytes).
			_ = *(*uint32)(unsafe.Pointer(auxCurrent))       // vna_hash
			_ = *(*uint16)(unsafe.Pointer(auxCurrent + 4))   // vna_flags
			vnaOther := *(*uint16)(unsafe.Pointer(auxCurrent + 6))
			vnaName := *(*uint32)(unsafe.Pointer(auxCurrent + 8))
			vnaNext := *(*uint32)(unsafe.Pointer(auxCurrent + 12))

			name := ReadCStringMem(strtabAddr, uintptr(vnaName))
			vt.Requirements[vnaOther] = &VersionRequirement{
				Name:  name,
				Index: vnaOther,
			}

			if vnaNext == 0 {
				break
			}
			auxCurrent += uintptr(vnaNext)
		}

		if vnNext == 0 {
			break
		}
		current += uintptr(vnNext)
	}
	return nil
}

// parseVerdef reads the Verdef chain.
// Each Verdef entry describes a version this library provides.
//
// Verdef structure:
//   uint16 vd_version  (version of structure, always 1)
//   uint16 vd_flags    (flags: VER_FLG_BASE if base version)
//   uint16 vd_ndx      (version index)
//   uint16 vd_cnt      (number of Verdaux entries)
//   uint32 vd_hash     (hash of version name)
//   uint32 vd_aux      (offset to first Verdaux)
//   uint32 vd_next     (offset to next Verdef, 0 if last)
//
// Verdaux structure:
//   uint32 vda_name    (string table offset)
//   uint32 vda_next    (offset to next Verdaux, 0 if last)
func (vt *VersionTable) parseVerdef(addr uintptr, count uint64, strtabAddr uintptr) error {
	current := addr
	for i := uint64(0); i < count; i++ {
		// Read Verdef header (20 bytes).
		vdVersion := *(*uint16)(unsafe.Pointer(current))
		_ = *(*uint16)(unsafe.Pointer(current + 2))  // vd_flags
		vdNdx := *(*uint16)(unsafe.Pointer(current + 4))
		_ = *(*uint16)(unsafe.Pointer(current + 6))  // vd_cnt
		_ = *(*uint32)(unsafe.Pointer(current + 8))  // vd_hash
		vdAux := *(*uint32)(unsafe.Pointer(current + 12))
		vdNext := *(*uint32)(unsafe.Pointer(current + 16))

		if vdVersion != 1 {
			return fmt.Errorf("unsupported verdef version %d", vdVersion)
		}

		// Read first Verdaux to get the version name.
		auxCurrent := current + uintptr(vdAux)
		vdaName := *(*uint32)(unsafe.Pointer(auxCurrent))

		name := ReadCStringMem(strtabAddr, uintptr(vdaName))
		vt.Definitions[vdNdx] = &VersionDefinition{
			Name:  name,
			Index: vdNdx,
		}

		if vdNext == 0 {
			break
		}
		current += uintptr(vdNext)
	}
	return nil
}

// GetSymbolVersion returns the version index for the given symbol index.
// Returns 0 if no version info is available, 1 for base/local version.
func (vt *VersionTable) GetSymbolVersion(symIdx uint32) uint16 {
	if vt.SymbolVersions == nil || uint64(symIdx) >= uint64(len(vt.SymbolVersions)) {
		return 0
	}
	return vt.SymbolVersions[symIdx] & 0x7fff // mask off hidden bit
}

// GetVersionName returns the human-readable version name for a version index.
// Returns empty string if the version is not found.
func (vt *VersionTable) GetVersionName(verIdx uint16) string {
	// Check requirements first (for dependency symbols).
	if req, ok := vt.Requirements[verIdx]; ok {
		return req.Name
	}
	// Then definitions (for exported symbols).
	if def, ok := vt.Definitions[verIdx]; ok {
		return def.Name
	}
	return ""
}

// ParseVersionInfo is a convenience function that parses version info from dynamic tags.
// It extracts the necessary addresses/counts from dynEntries and calls ParseVersionTables.
func ParseVersionInfo(dynEntries map[elf.DynTag]uint64, baseAddr, strtabAddr uintptr, symCount uint64) (*VersionTable, error) {
	vt := NewVersionTable()

	versymAddr := uintptr(0)
	if v, ok := dynEntries[elf.DT_VERSYM]; ok {
		versymAddr = baseAddr + uintptr(v)
	}

	verneedAddr := uintptr(0)
	verneedNum := uint64(0)
	if v, ok := dynEntries[elf.DT_VERNEED]; ok {
		verneedAddr = baseAddr + uintptr(v)
	}
	if v, ok := dynEntries[elf.DT_VERNEEDNUM]; ok {
		verneedNum = v
	}

	verdefAddr := uintptr(0)
	verdefNum := uint64(0)
	if v, ok := dynEntries[elf.DT_VERDEF]; ok {
		verdefAddr = baseAddr + uintptr(v)
	}
	if v, ok := dynEntries[elf.DT_VERDEFNUM]; ok {
		verdefNum = v
	}

	if err := vt.ParseVersionTables(
		versymAddr, symCount,
		verneedAddr, verneedNum,
		verdefAddr, verdefNum,
		strtabAddr,
	); err != nil {
		return nil, err
	}

	return vt, nil
}

// ParseDynTags extracts DT_VERSYM, DT_VERNEED, DT_VERNEEDNUM, DT_VERDEF, DT_VERDEFNUM
// from the raw dynamic section bytes.
func ParseDynTags(dynData []byte) map[elf.DynTag]uint64 {
	tags := make(map[elf.DynTag]uint64)
	const dynEntSize = 16
	for off := 0; off+dynEntSize <= len(dynData); off += dynEntSize {
		tag := elf.DynTag(binary.LittleEndian.Uint64(dynData[off:]))
		val := binary.LittleEndian.Uint64(dynData[off+8:])
		if tag == elf.DT_NULL {
			break
		}
		tags[tag] = val
	}
	return tags
}
