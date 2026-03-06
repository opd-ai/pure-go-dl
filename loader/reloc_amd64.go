//go:build amd64 || (linux && !arm64)

package loader

// x86-64 relocation types (System V ABI supplement).
const (
	R_X86_64_NONE      = 0
	R_X86_64_64        = 1
	R_X86_64_PC32      = 2
	R_X86_64_GOT32     = 3
	R_X86_64_PLT32     = 4
	R_X86_64_COPY      = 5
	R_X86_64_GLOB_DAT  = 6
	R_X86_64_JUMP_SLOT = 7
	R_X86_64_RELATIVE  = 8
	R_X86_64_GOTPCREL  = 9
	R_X86_64_32        = 10
	R_X86_64_32S       = 11
	R_X86_64_16        = 12
	R_X86_64_PC16      = 13
	R_X86_64_8         = 14
	R_X86_64_PC8       = 15
	R_X86_64_DTPMOD64  = 16
	R_X86_64_DTPOFF64  = 17
	R_X86_64_TPOFF64   = 18
	R_X86_64_TLSGD     = 19
	R_X86_64_TLSLD     = 20
	R_X86_64_DTPOFF32  = 21
	R_X86_64_GOTTPOFF  = 22
	R_X86_64_TPOFF32   = 23
	R_X86_64_PC64      = 24
	R_X86_64_GOTOFF64  = 25
	R_X86_64_GOTPC32   = 26
	R_X86_64_SIZE32    = 32
	R_X86_64_SIZE64    = 33
	R_X86_64_IRELATIVE = 37
)

// relaEntSize is the size in bytes of an Elf64_Rela record.
const relaEntSize = 24

func relaSymIdx(info uint64) uint32 { return uint32(info >> 32) }
func relaType(info uint64) uint32   { return uint32(info & 0xffffffff) }

const (
	relocNone      = R_X86_64_NONE
	relocRelative  = R_X86_64_RELATIVE
	reloc64        = R_X86_64_64
	relocGlobDat   = R_X86_64_GLOB_DAT
	relocJumpSlot  = R_X86_64_JUMP_SLOT
	relocCopy      = R_X86_64_COPY
	reloc32        = R_X86_64_32
	reloc32S       = R_X86_64_32S
	relocPC32      = R_X86_64_PC32
	relocPLT32     = R_X86_64_PLT32
	relocDTPMod64  = R_X86_64_DTPMOD64
	relocDTPOff64  = R_X86_64_DTPOFF64
	relocTPOff64   = R_X86_64_TPOFF64
	relocDTPOff32  = R_X86_64_DTPOFF32
	relocTPOff32   = R_X86_64_TPOFF32
	relocGOTTPOff  = R_X86_64_GOTTPOFF
	relocTLSGD     = R_X86_64_TLSGD
	relocTLSLD     = R_X86_64_TLSLD
	relocIRelative = R_X86_64_IRELATIVE
)
