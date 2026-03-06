# pure-go-dl

A CGO-free ELF dynamic linker that enables loading native shared libraries (`.so` files) from Go binaries built with `CGO_ENABLED=0`.

## Overview

**pure-go-dl** provides `dlopen`/`dlsym`/`dlclose` semantics for x86-64 and ARM64 Linux without requiring cgo at build time. It allows Go applications to load and call native C libraries at runtime while avoiding the complexity of cgo compilation.

**Important:** While this library is built with `CGO_ENABLED=0` (no cgo compiler required), the resulting binaries are **not statically linked**. They require standard system libraries (`libc.so.6`, `libdl.so.2`, `libpthread.so.0`) at runtime due to the [purego](https://github.com/ebitengine/purego) dependency which uses assembly trampolines to call system `dlopen`/`dlsym`.

**Key Features:**
- ✅ Load ELF shared objects from `CGO_ENABLED=0` binaries
- ✅ Symbol resolution with GNU hash and SysV hash support
- ✅ Full relocation processing (RELATIVE, GLOB_DAT, JUMP_SLOT, COPY)
- ✅ Transitive dependency loading (handles `DT_NEEDED`)
- ✅ Constructor/destructor execution (`DT_INIT`, `DT_FINI`, init/fini arrays)
- ✅ Reference counting for proper cleanup
- ✅ Symbol versioning support (`DT_VERSYM`, `DT_VERDEF`, `DT_VERNEED`)
- ✅ Library search paths (`DT_RUNPATH`, `DT_RPATH`, `LD_LIBRARY_PATH`, `/etc/ld.so.cache`)

**Why This Exists:**
- Build Go applications without the cgo compiler while still loading platform-specific libraries (GPU drivers, system libraries)
- Avoid cgo cross-compilation complexity in containerized/embedded build environments
- Enable runtime plugin systems in `CGO_ENABLED=0` binaries
- Simplify builds for environments where cgo is unavailable or problematic

## Installation

```bash
go get github.com/opd-ai/pure-go-dl
```

**Build Requirements:**
- Go 1.24 or later
- `CGO_ENABLED=0` (no cgo compiler needed)

**Runtime Requirements:**
- Linux (x86-64 or ARM64/aarch64)
- Standard system libraries: `libc.so.6`, `libdl.so.2`, `libpthread.so.0`
- Note: Binaries are **dynamically linked** despite being built with `CGO_ENABLED=0`

## Quick Start

### Basic Usage

```go
package main

import (
    "fmt"
    "github.com/opd-ai/pure-go-dl/dl"
)

func main() {
    // Load a shared library
    lib, err := dl.Open("libm.so.6")
    if err != nil {
        panic(err)
    }
    defer lib.Close()

    // Bind a function to a Go variable
    var cos func(float64) float64
    if err := lib.Bind("cos", &cos); err != nil {
        panic(err)
    }

    // Call the native function
    fmt.Println(cos(0)) // Output: 1.0
}
```

### Loading Custom Libraries

```go
// Load a library with dependencies
lib, err := dl.Open("/path/to/libcustom.so", dl.RTLD_GLOBAL)
if err != nil {
    panic(err)
}
defer lib.Close()

// Look up a symbol address
addr, err := lib.Sym("my_function")
if err != nil {
    panic(err)
}
fmt.Printf("my_function address: %#x\n", addr)

// Or bind and call directly
var myFunc func(int, int) int
lib.Bind("my_function", &myFunc)
result := myFunc(10, 20)
```

### Command-Line Tool

The `pgldd` utility loads a shared library and prints its symbol table:

```bash
go install github.com/opd-ai/pure-go-dl/cmd/pgldd@latest
pgldd /lib/x86_64-linux-gnu/libm.so.6
```

## API Reference

### Opening Libraries

```go
func Open(name string, flags ...Flag) (*Library, error)
```

Load a shared library by name or path. Supports:
- `RTLD_LOCAL` (default): Symbols are only visible to this library and its dependents
- `RTLD_GLOBAL`: Symbols are visible to all subsequently loaded libraries
- `RTLD_NOW`: Immediate symbol binding (compatibility flag; all libraries use eager binding)

Note: `RTLD_LAZY` (deferred binding) is not supported. All symbol binding is eager.

### Symbol Resolution

```go
func (l *Library) Sym(name string) (uintptr, error)
```

Returns the address of an exported symbol.

```go
func (l *Library) Bind(name string, fnPtr interface{}) error
```

Resolves a symbol and binds it to a Go function variable using [purego](https://github.com/ebitengine/purego). The `fnPtr` must be a pointer to a function variable with the correct signature.

### Cleanup

```go
func (l *Library) Close() error
```

Decrements reference count and unloads the library when it reaches zero. Destructors are run in reverse dependency order.

## Current Status

### Completed Milestones

- ✅ **M0: Foundations** — Syscall helpers, repository structure
- ✅ **M1: Memory Map** — PT_LOAD segment mapping, BSS zero-fill
- ✅ **M2: Symbols** — GNU hash and SysV hash lookup
- ✅ **M3: Relocations** — x86-64 and ARM64 relocation types (RELATIVE, GLOB_DAT, JUMP_SLOT, COPY, etc.)
- ✅ **M4: First Call** — Successfully calls native functions from CGO_ENABLED=0 binary
- ✅ **M5: Dependencies** — Transitive dependency loading via DT_NEEDED
- ✅ **M6: Init/Fini** — Constructor/destructor execution in correct order
- ✅ **M7: Versioning** — GNU symbol versioning support
- ✅ **M7.2: IFUNC** — Indirect function (IFUNC) resolution for CPU-optimized functions
- ✅ **M7.3: TLS** — Full multi-threaded Thread-Local Storage support with Dynamic Thread Vector (DTV)
- ✅ **M7.4: ARM64 Port** — Full aarch64/ARM64 architecture support for Linux

### Integration Tests Passing

All tests run successfully with `CGO_ENABLED=0 go test ./...`:
- Loading shared libraries and dependency chains
- Symbol lookup and function binding
- Calling native C functions with correct results
- Constructor side effects (initialization)
- Internal relocations (library calling its own functions)
- Reference counting and cleanup
- Weak symbol handling
- Multi-threaded TLS access with per-thread isolation

## Limitations

### Runtime Dependencies

Despite being built with `CGO_ENABLED=0`, binaries using pure-go-dl are **not statically linked**. They require the following system libraries at runtime:

- `libc.so.6` (glibc)
- `libdl.so.2` (dynamic linker interface)
- `libpthread.so.0` (POSIX threads)

This is due to the [purego](https://github.com/ebitengine/purego) dependency, which uses assembly trampolines to invoke system `dlopen`/`dlsym` calls. While you avoid the cgo **compiler** dependency (simplifying builds), you still require a standard Linux runtime environment.

**What this means for deployment:**
- ✅ No need for cgo during builds (simpler CI/CD, cross-compilation)
- ✅ No C compiler toolchain required in build environment
- ❌ Cannot deploy to environments without glibc (e.g., Alpine Linux without glibc compatibility)
- ❌ Binaries are not "single-file standalone" — they need system libraries

If you need truly static binaries, consider using cgo with static linking flags instead.

### Not Yet Supported

- **Lazy Binding**: Only eager binding (`RTLD_NOW` semantics) is implemented. `RTLD_LAZY` is explicitly a non-goal.

### Library Compatibility

The loader successfully handles:
- ✅ libm.so.6 (math library)
- ✅ libz.so (compression)
- ✅ Most glibc-based system libraries
- ✅ Custom libraries compiled with `-fPIC -shared`
- ✅ Libraries using IFUNC (GNU indirect functions for CPU-optimized variants)
- ✅ Libraries using TLS (Thread-Local Storage) with multi-threading support

## Testing

Run the test suite:

```bash
# Build test libraries
make -C testdata

# Run all tests with race detector
CGO_ENABLED=0 go test -race ./...

# Run specific package tests
go test -v ./dl/
```

The `testdata/` directory contains sample C libraries used for integration testing:
- `libtest.so`: Simple add/square functions with constructor
- `libreloc.so`: Tests internal function calls and relocations

## Development

### Project Structure

```
pure-go-dl/
├── dl/          # Public API (Open, Sym, Bind, Close)
├── elf/         # ELF parsing beyond stdlib debug/elf
├── loader/      # Core loading, mapping, relocation
├── symbol/      # Symbol tables, hashing, lookup, versioning
├── internal/
│   └── mmap/    # Memory mapping syscall helpers
├── cmd/
│   └── pgldd/   # CLI tool for symbol inspection
└── testdata/    # Test shared libraries
```

### Building

```bash
# Build all packages
go build ./...

# Build CLI tool
go build -o pgldd ./cmd/pgldd

# Verify CGO_ENABLED=0 works (produces dynamically-linked binary)
CGO_ENABLED=0 go build -o pgldd-nocgo ./cmd/pgldd
file pgldd-nocgo  # Shows "dynamically linked" (expected due to purego)
```

## Roadmap

See [ROADMAP.md](ROADMAP.md) for detailed implementation phases and future features.

**Recently completed:**
- ✅ ARM64/aarch64 port (MEDIUM priority — needed for Linux ARM servers and container-free deployment)
- ✅ Multi-threaded TLS support with Dynamic Thread Vector (DTV) (MEDIUM priority — needed for pthread-heavy libraries)
- ✅ IFUNC resolution support (HIGH priority — needed for glibc optimized functions)
- ✅ Symbol versioning (HIGH priority — needed for libc dependencies)

**Non-goals:**
- Lazy binding / PLT trampolines
- `LD_AUDIT` or `LD_PRELOAD` semantics
- Windows PE or macOS Mach-O support

## Contributing

Contributions are welcome! Please:
1. Read [ROADMAP.md](ROADMAP.md) for project context and design decisions
2. Check existing issues and milestones
3. Write integration tests for new features
4. Ensure `CGO_ENABLED=0 go test -race ./...` passes

## License

See [LICENSE](LICENSE) file.

## References

- [ELF Specification](https://refspecs.linuxfoundation.org/elf/elf.pdf)
- [System V AMD64 ABI](https://refspecs.linuxfoundation.org/elf/x86_64-abi-0.99.pdf)
- [glibc dynamic linker source](https://sourceware.org/git/?p=glibc.git;a=tree;f=elf)
- [purego - Call C functions from Go without cgo](https://github.com/ebitengine/purego)
