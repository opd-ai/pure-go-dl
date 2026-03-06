// Package loader implements core ELF shared object loading for pure Go applications.
//
// The loader handles memory mapping of PT_LOAD segments, relocation processing
// (including x86-64 and ARM64 architectures), and constructor/destructor execution.
// It provides the low-level operations needed to map a shared library into memory
// and make it executable without requiring CGO.
//
// # Loading Process
//
// The Load function performs these steps:
//   - Memory maps PT_LOAD segments from the ELF file
//   - Zero-fills BSS sections
//   - Processes relocations (RELATIVE, GLOB_DAT, JUMP_SLOT, COPY, TLS)
//   - Handles IFUNC (indirect function) resolution
//   - Allocates TLS blocks for thread-local storage
//   - Executes DT_INIT and DT_INIT_ARRAY constructors
//
// # Unloading Process
//
// The Unload function reverses the loading process:
//   - Executes DT_FINI and DT_FINI_ARRAY destructors (in reverse order)
//   - Releases TLS blocks
//   - Unmaps all memory segments
//
// # Thread Safety
//
// The loader is safe for concurrent use. Memory mapping and unmapping operations
// are atomic at the syscall level, and TLS management uses proper synchronization.
package loader
