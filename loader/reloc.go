package loader

import "github.com/opd-ai/pure-go-dl/symbol"

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

// relaEntry mirrors Elf64_Rela: offset(8) + info(8) + addend(8).
type relaEntry struct {
	Offset uint64
	Info   uint64
	Addend int64
}

func relaSymIdx(info uint64) uint32 { return uint32(info >> 32) }
func relaType(info uint64) uint32   { return uint32(info & 0xffffffff) }

func symName(obj *Object, idx uint32) string {
	if idx == 0 || obj.SymtabAddr == 0 || obj.StrtabAddr == 0 {
		return ""
	}
	sym := (*symbol.Elf64Sym)(unsafePointer(obj.SymtabAddr + uintptr(idx)*24))
	return symbol.ReadCStringMem(obj.StrtabAddr, uintptr(sym.Name))
}
