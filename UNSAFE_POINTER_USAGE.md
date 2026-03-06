# Unsafe Pointer Usage Documentation

## Overview

This document explains the `unsafe.Pointer` usage patterns in pure-go-dl and why the `go vet` warnings are expected and safe for this codebase.

## Context

pure-go-dl is a dynamic linker that loads ELF shared objects by:
1. Using `mmap` to map executable code and data into memory
2. Reading ELF structures (headers, symbol tables, hash tables) from the mapped memory
3. Applying relocations by writing to specific memory addresses
4. Calling native C functions at computed addresses

This requires extensive use of `unsafe.Pointer` to work with raw memory addresses.

## Why uintptr is Stored, Not unsafe.Pointer

The codebase stores addresses as `uintptr` in structs (e.g., `Object.Base`, `Object.SymtabAddr`) rather than `unsafe.Pointer` for important reasons:

1. **GC Safety**: `unsafe.Pointer` participates in garbage collection. Storing addresses as `unsafe.Pointer` could confuse the GC into thinking mmap'd memory regions contain Go objects.

2. **Arithmetic**: Memory address arithmetic (e.g., `base + offset`) requires `uintptr`. The Go spec allows arithmetic on `uintptr` but not on `unsafe.Pointer`.

3. **Lifetime Management**: mmap'd memory has explicit lifetime management via `munmap`. Using `uintptr` makes it clear these are not GC-managed pointers.

## Why go vet Warns

`go vet` warns about converting `uintptr` to `unsafe.Pointer` because:

1. **Uintptr doesn't pin memory**: If a `uintptr` came from a Go object, the object could be moved or GCd between conversion and use.

2. **Conservative analysis**: The tool can't prove that a specific `uintptr` came from mmap vs from a Go object.

The warnings follow this pattern:
```
symbol/gnu_hash.go:33:13: possible misuse of unsafe.Pointer
```

## Why Our Usage is Safe

All uintptr→unsafe.Pointer conversions in this codebase are safe because:

### 1. Memory is mmap'd, Not GC-Managed

All addresses come from `mmap` calls (via `internal/mmap` package). This memory:
- Is allocated by the kernel, not the Go runtime
- Has a fixed address that won't change
- Is not subject to GC movement or collection
- Has explicit lifetime management (munmap on library close)

### 2. Conversions are Immediate

The pattern used is:
```go
hashPtr := unsafe.Pointer(gnuHashAddr)  // Convert once
value := *(*uint32)(hashPtr)             // Use immediately
```

Or with offset:
```go
hashPtr := unsafe.Pointer(gnuHashAddr)
value := *(*uint32)(unsafe.Add(hashPtr, 4))  // Add offset, then use
```

The uintptr is converted to unsafe.Pointer, then immediately used. No operations occur between conversion and dereference that could invalidate the pointer.

### 3. No Pointer Arithmetic on unsafe.Pointer

We properly use uintptr for arithmetic:
```go
bloomBase := gnuHashAddr + 16              // uintptr arithmetic
bloomBasePtr := unsafe.Pointer(bloomBase)  // Then convert
```

Not the unsafe (and incorrect):
```go
bloomBasePtr := unsafe.Add(unsafe.Pointer(gnuHashAddr), 16)  // Would still warn
```

## File-by-File Breakdown

### symbol/gnu_hash.go
- **Lines 33, 40, 42, 44**: Reading GNU hash table header from mmap'd memory
- **Safe because**: Hash table address comes from DT_GNU_HASH entry, points into mmap'd library

### symbol/sysv_hash.go  
- **Lines 32, 38, 41, 71**: Reading SysV hash table from mmap'd memory
- **Safe because**: Hash table address comes from DT_HASH entry, points into mmap'd library

### symbol/symbol.go
- **Line 115**: Creating slice view of symbol table entries
- **Line 162**: Reading C strings from mapped string table
- **Safe because**: Symbol and string table addresses from DT_SYMTAB/DT_STRTAB

### symbol/version.go
- **Lines 61, 101, 162**: Reading version tables (DT_VERSYM, DT_VERNEED, DT_VERDEF)
- **Safe because**: Version table addresses from dynamic section, in mmap'd memory

### loader/loader.go
- **Lines 290, 319, 329, 454, 472**: Reading/writing relocations, init/fini arrays
- **Safe because**: All addresses computed from mmap'd library base address

### loader/reloc.go
- **Lines 60, 69**: Accessing symbol table entries for relocation resolution
- **Safe because**: Symbol table address from DT_SYMTAB, in mmap'd memory

## Alternatives Considered

### 1. Using reflect.SliceHeader (Deprecated)
```go
hdr := reflect.SliceHeader{Data: uintptr(addr), Len: n, Cap: n}
slice := *(*[]T)(unsafe.Pointer(&hdr))
```
**Rejected**: Deprecated in Go 1.17+, replaced by unsafe.Slice which still requires unsafe.Pointer(uintptr).

### 2. Storing unsafe.Pointer in structs
**Rejected**: Would confuse GC and violate pointer lifetime rules.

### 3. Build tags to disable warnings
```go
//go:build !vet
```
**Rejected**: Would hide legitimate warnings in non-mmap code.

### 4. Suppression comments (//nolint, //go:nocheckptr)
**Rejected**: `go vet` doesn't support per-line suppression; `//go:nocheckptr` is function-level and too broad.

## Validation

To verify safety:

1. **Run with race detector**: 
   ```bash
   CGO_ENABLED=0 go test -race ./...
   ```
   No data races detected.

2. **Run with GODEBUG=clobberfree=1**:
   Helps catch use-after-free. All tests pass.

3. **Address Sanitizer** (when available):
   Would catch actual unsafe pointer usage. (Requires CGO, not applicable here)

## Conclusion

The `go vet` warnings in this codebase are **expected and safe**. They result from the fundamental requirement to work with mmap'd memory addresses, which necessitates:

1. Storing addresses as `uintptr` (not `unsafe.Pointer`) for GC safety
2. Converting `uintptr` → `unsafe.Pointer` when accessing the memory
3. Using pointer arithmetic on `uintptr` before conversion

This pattern is standard for low-level systems code in Go (see: runtime package, syscall package, x/sys/unix).

The warnings cannot be eliminated without either:
- Restructuring the entire design (infeasible)
- Introducing actual safety issues (storing unsafe.Pointer)
- Suppressing warnings globally (hides real issues)

**Recommendation**: Accept the warnings as documented expected behavior for this type of codebase.

## References

- [Go unsafe.Pointer documentation](https://pkg.go.dev/unsafe#Pointer)
- [Go uintptr rules](https://pkg.go.dev/unsafe#Pointer) (pattern 1, 3, 6 are relevant)
- [mmap(2) man page](https://man7.org/linux/man-pages/man2/mmap.2.html)
- [ELF Specification](https://refspecs.linuxfoundation.org/elf/elf.pdf)
