# pure-go-dl: A Pure-Go ELF Dynamic Linker

## Project Plan

### Goals

- Load ELF shared objects (.so) from a `CGO_ENABLED=0` Go binary
- Resolve symbols to callable function pointers
- Handle transitive dependencies
- Provide `dlopen`/`dlsym`/`dlclose` semantics
- Target x86-64 Linux initially, design for portability

### Non-Goals (Initially)

- TLS (`__thread` / `_Thread_local`) support
- IFUNC resolution
- Lazy binding (PLT/GOT deferred resolution)
- `LD_AUDIT`, `LD_PRELOAD` semantics
- Architectures other than x86-64

---

## Phase 0: Foundations and Research

**Duration: 1–2 weeks**

Before writing any loader code, build confidence in the primitives.

### 0.1 Study the ELF Specification

Read the following in order:

1. ELF spec chapters on program headers, dynamic segment, and relocations
2. The LSB (Linux Standard Base) x86-64 ABI supplement — specifically
   the relocation types table
3. `glibc/elf/dl-load.c` and `musl/ldso/dynlink.c` — not to replicate,
   but to understand sequencing decisions

Produce a personal reference document mapping ELF structures to
Go's `debug/elf` types. Note where `debug/elf` has gaps (it does).

### 0.2 Study purego

Clone `github.com/ebitengine/purego` and read its assembly trampolines
and `RegisterFunc` internals. Understand how it transitions from a
goroutine stack to a system-stack C call. You will depend on or adapt
this work — do not rewrite it from scratch without good reason.

### 0.3 Build a Minimal Syscall Toolkit

Write thin wrappers around the syscalls you'll need. Confirm each one
works in a `CGO_ENABLED=0` binary:

- `mmap` (with `PROT_READ|PROT_WRITE|PROT_EXEC`, `MAP_PRIVATE|MAP_ANONYMOUS`)
- `mprotect`
- `munmap`
- `open`, `read`, `close`, `fstat` (or use `os` package)

Use `golang.org/x/sys/unix` — do not hand-roll `syscall.Syscall6` unless
the package is insufficient.

### 0.4 Decide on Module Boundaries

Create the repository with this package layout:

```
pure-go-dl/
├── elf/         # ELF parsing beyond debug/elf
├── loader/      # Core loading, mapping, relocation
├── symbol/      # Symbol tables, hashing, lookup
├── runtime/     # Calling convention bridges
├── dl/          # Public API: Open, Sym, Close
├── cmd/
│   └── pgldd/   # CLI tool: load a .so and print its symbol table
└── internal/
    └── mmap/    # Syscall helpers
```

This layout separates concerns early and prevents the loader from
becoming monolithic — which is the very problem the article warns about.

---

## Phase 1: Load and Map a Single Shared Object

**Duration: 2–3 weeks**

The goal is to take a path to a `.so` file and produce a correct
memory image. No symbol resolution yet.

### 1.1 Parse ELF Headers

Use `debug/elf` to open the file and read:

- ELF header (confirm ET_DYN, EM_X86_64, ELFCLASS64)
- Program headers (filter to PT_LOAD, PT_DYNAMIC, PT_GNU_RELRO)
- Dynamic section entries (DT_NEEDED, DT_SYMTAB, DT_STRTAB,
  DT_RELA, DT_RELASZ, DT_HASH, DT_GNU_HASH, etc.)

Write a struct that captures all of this in loader-friendly form.

### 1.2 Compute the Memory Layout

Iterate PT_LOAD segments to determine:

- Total address span (max vaddr+memsz − min vaddr), page-aligned
- Per-segment offset within that span
- Per-segment protection flags (PF_R, PF_W, PF_X → PROT_*)

### 1.3 Map the Object

1. Reserve the full span with a single `mmap` call
   (`PROT_NONE`, `MAP_PRIVATE|MAP_ANONYMOUS`) to get a contiguous
   base address.
2. For each PT_LOAD segment, `mmap` the file's contents at the
   correct offset within the reservation using `MAP_FIXED`.
3. Zero-fill any BSS regions (memsz > filesz).
4. Apply `mprotect` to set final permissions per segment.

Record the base address — all relocations are relative to it.

### 1.4 Verification Checkpoint

Write a test that loads a trivial `.so` (compile one with
`gcc -shared -o libtest.so test.c` containing a single exported
global integer), memory-maps it, and reads the global's value
by manually computing its address from the symbol table.

Do not proceed until this works reliably.

---

## Phase 2: Symbol Table and Lookup

**Duration: 1–2 weeks**

### 2.1 Parse the Symbol Table

From the dynamic section, locate `DT_SYMTAB` and `DT_STRTAB`.
Read `Elf64_Sym` entries. Build a Go map from symbol name to
`Symbol` struct:

```go
type Symbol struct {
    Name    string
    Value   uintptr // base + sym.st_value
    Size    uint64
    Bind    elf.SymBind
    Type    elf.SymType
    Section elf.SectionIndex
}
```

### 2.2 Implement SysV Hash Lookup

Parse the `DT_HASH` table. Implement the SysV hash function and
bucket/chain walk. This is simple and specified exactly in the ELF
spec. Use it as your primary lookup path first.

### 2.3 Implement GNU Hash Lookup

Parse the `DT_GNU_HASH` table. Implement the GNU hash algorithm
with Bloom filter check, bucket lookup, and chain walk. Most real
shared libraries use GNU hash exclusively (DT_HASH is sometimes
absent). This must work before you can load system libraries.

### 2.4 Verification Checkpoint

Load `libm.so.6`, look up `cos` by name, confirm you get a
non-zero address. Don't call it yet — just confirm resolution.

---

## Phase 3: Relocations

**Duration: 2–3 weeks**

This is the most architecture-specific and error-prone phase.

### 3.1 Process DT_RELA Relocations

Parse the `DT_RELA` table (x86-64 uses RELA, not REL — addends
are explicit). For each entry, dispatch on `r_info` type:

| Type                      | Operation                          | Priority |
|---------------------------|------------------------------------|----------|
| R_X86_64_RELATIVE         | `*(base + offset) = base + addend` | Do first |
| R_X86_64_64               | `*(base + offset) = S + addend`    | High     |
| R_X86_64_GLOB_DAT         | `*(base + offset) = S`             | High     |
| R_X86_64_JUMP_SLOT        | `*(base + offset) = S`             | High     |
| R_X86_64_COPY             | `memcpy(offset, &S, size)`         | Medium   |
| R_X86_64_DTPMOD64         | Skip (TLS — Phase 7)              | Deferred |
| R_X86_64_DTPOFF64         | Skip (TLS — Phase 7)              | Deferred |
| R_X86_64_TPOFF64          | Skip (TLS — Phase 7)              | Deferred |
| R_X86_64_IRELATIVE        | Skip (IFUNC — Phase 7)            | Deferred |

`S` means "resolved symbol value." If resolution fails, record
the error with full context (library, symbol name, relocation
type, offset).

### 3.2 Process DT_JMPREL (PLT Relocations)

These are R_X86_64_JUMP_SLOT entries in a separate table. Process
them identically to DT_RELA JUMP_SLOT entries (eager binding).
Lazy binding is a non-goal for now.

### 3.3 Apply GNU RELRO

If PT_GNU_RELRO is present, after all relocations are applied,
`mprotect` the indicated range to `PROT_READ`. This is important
for correctness — some libraries assume RELRO is enforced.

### 3.4 Verification Checkpoint

Load a `.so` that exports a function calling another function
within the same library (internal PLT call). Verify all
relocations resolve. Inspect the GOT entries manually to confirm
correct addresses.

Example test library:

```c
// libreloc.c — compile with: gcc -shared -fPIC -o libreloc.so libreloc.c
static int square(int x) { return x * x; }
int square_plus_one(int x) { return square(x) + 1; }
```

---

## Phase 4: Calling Loaded Code

**Duration: 1–2 weeks**

### 4.1 Integrate purego's Calling Bridge

Add `github.com/ebitengine/purego` as a dependency. Use its
`RegisterFunc` to bind a resolved symbol address to a Go function
variable:

```go
var cos func(float64) float64
purego.RegisterFunc(&cos, cosAddr)
fmt.Println(cos(0)) // 1.0
```

If purego's approach doesn't fit (e.g., you need variadic calls
or struct-by-value), write custom assembly trampolines. But start
with purego.

### 4.2 Raw Call Fallback

Provide a lower-level `Call(addr uintptr, args ...uintptr) uintptr`
for cases where the caller knows the ABI and just wants to invoke
an address. This is a thin wrapper around an assembly trampoline
that saves/restores Go state, switches to the system stack, and
issues the call per the SysV AMD64 ABI.

### 4.3 Stack Considerations

Go goroutine stacks start small (a few KB) and grow. C code
expects at minimum 2 MB. The trampoline must switch to a stack
that satisfies this — purego handles this, but verify it with
deeply recursive or stack-heavy C functions.

### 4.4 Verification Checkpoint

This is the milestone that proves the project works end-to-end:

```go
lib, _ := dl.Open("libm.so.6")
var cos func(float64) float64
lib.Bind("cos", &cos)
fmt.Println(cos(3.14159265)) // ≈ -1.0
lib.Close()
```

This must run in a `CGO_ENABLED=0` binary. If it does, announce the
project — this is already more useful than what exists today.

---

## Phase 5: Transitive Dependency Resolution

**Duration: 2–3 weeks**

### 5.1 Library Search Paths

Implement the search order:

1. `DT_RUNPATH` from the requesting library
2. `LD_LIBRARY_PATH` environment variable
3. `/etc/ld.so.cache` (parse the binary cache format — it's a
   simple header + array of string pairs)
4. Default paths: `/lib/x86_64-linux-gnu`, `/usr/lib/x86_64-linux-gnu`,
   `/lib64`, `/usr/lib64`, `/lib`, `/usr/lib`

`DT_RPATH` is deprecated but still encountered — support it with
lower priority than `DT_RUNPATH`.

### 5.2 Dependency Graph Construction

When loading a library, read its `DT_NEEDED` entries. For each:

1. Check if already loaded (maintain a global map of soname → handle)
2. If not, search, load, and map it (Phase 1)
3. Recurse

Use a topological sort to determine initialization order. Handle
circular dependencies by detecting back-edges and breaking cycles
(in practice, circular deps are rare but glibc ↔ libpthread is one).

### 5.3 Symbol Resolution Across Libraries

When a relocation references an undefined symbol, search the
dependency graph in breadth-first order (matching ld-linux behavior).
Implement `RTLD_GLOBAL` vs `RTLD_LOCAL` scoping: global libraries
are visible to all future lookups, local ones only to their
dependents.

### 5.4 Verification Checkpoint

Load a library with a non-trivial dependency chain. Good test case:
load `libcurl.so` (depends on libssl, libcrypto, libz, libc, etc.).
Resolve `curl_easy_init` and call it. If it returns a non-null
handle, the entire dependency chain loaded and initialized correctly.

---

## Phase 6: Initialization and Teardown

**Duration: 1–2 weeks**

### 6.1 Run Constructors

After all relocations for a library (and its dependencies) are
complete, execute:

1. The `DT_INIT` function (if present) — a single function pointer
2. The `DT_INIT_ARRAY` functions (if present) — call each in order

Constructors must run in dependency order: dependencies before
dependents.

### 6.2 Run Destructors on Close

When closing a library (and its refcount hits zero):

1. Run `DT_FINI_ARRAY` in reverse order
2. Run `DT_FINI` (if present)
3. `munmap` all segments
4. Remove from the loaded library map

Destructors run in reverse dependency order.

### 6.3 Reference Counting

`dl.Open` on an already-loaded soname increments the refcount and
returns the existing handle. `dl.Close` decrements it. Only unload
when it reaches zero. This matches `dlopen`/`dlclose` semantics.

### 6.4 Verification Checkpoint

Write a test `.so` whose constructor writes to a shared memory
flag and whose destructor clears it. Confirm the flag is set after
`Open` and cleared after `Close`.

---

## Phase 7: Advanced Features (Post-Launch)

Each of these is independently valuable. Prioritize based on what
real-world libraries you need to load.

### 7.1 Symbol Versioning

Many system libraries (especially glibc's `libc.so.6`) use GNU
symbol versioning. Without this, you cannot correctly resolve
symbols like `stat` which have multiple versions. Parse
`DT_VERSYM`, `DT_VERDEF`, `DT_VERNEED`. When a symbol has a
version requirement, match it during lookup.

**Priority: High** — needed for loading anything that depends on libc.

### 7.2 IFUNC Resolution ✅ COMPLETE

`STT_GNU_IFUNC` symbols point to a resolver function that must
be called to obtain the real address. The resolver typically
checks CPU features (via `getauxval(AT_HWCAP)`) and returns an
optimized implementation. You must call native code during
loading — use your Phase 4 calling bridge.

**Priority: High** — glibc uses IFUNC extensively for string and
memory functions.

**Status: COMPLETE** — Implemented in commit 5c7ba99:
- R_X86_64_IRELATIVE relocation handling in loader
- STT_GNU_IFUNC symbol resolution in dl package
- CallIfuncResolver() using purego for C ABI compliance
- Comprehensive tests with libifunc.so test library

### 7.3 TLS Support ⚠️ PARTIAL

Thread-Local Storage (TLS) support is partially implemented:

**Completed:**
- ✅ PT_TLS segment parsing and detection
- ✅ TLS module registration and management
- ✅ TLS block allocation with proper initialization (data + BSS)
- ✅ R_X86_64_DTPMOD64 relocation (module ID)
- ✅ R_X86_64_DTPOFF64 relocation (module-relative offset)
- ✅ R_X86_64_TPOFF64 relocation (thread pointer offset)
- ✅ R_X86_64_DTPOFF32 and R_X86_64_TPOFF32 (32-bit variants)
- ✅ Comprehensive TLS infrastructure and tests
- ✅ `__tls_get_addr` runtime function for dynamic TLS access (using purego.NewCallback)
- ✅ TLS initialization data mapping (fixed page alignment bug)

**Not Yet Implemented:**
- ❌ R_X86_64_TLSGD, R_X86_64_TLSLD, R_X86_64_GOTTPOFF (code sequence relocations)
- ❌ Per-thread TLS block management (currently single-threaded)
- ❌ Dynamic Thread Vector (DTV) for multi-threaded access

**Current Status:**

Libraries with `PT_TLS` segments can be loaded and executed successfully. TLS variables
can be accessed and modified through functions that use `__tls_get_addr`. The General
Dynamic (GD) TLS access model is now supported via a C-callable callback created with
purego.NewCallback.

**Remaining Limitations:**

1. **Single-threaded only**: All TLS accesses use a pseudo thread ID (always 1). True
   per-thread storage would require gettid() syscall integration and runtime cooperation.

2. **Code sequence relocations**: R_X86_64_TLSGD, TLSLD, and GOTTPOFF require rewriting
   instruction sequences, which is complex and not yet implemented. Libraries using
   these relocations will fail to load with clear error messages.

**Workarounds:**

1. Most libraries use the Initial Exec or Local Exec TLS models, which are fully supported
2. For code sequence relocations, use libraries compiled with `-ftls-model=initial-exec`
3. Single-threaded use cases work correctly

**Priority: Medium** — needed for pthread-heavy libraries and newer system libraries.

**Status: PARTIAL** — Implemented across commits:
- internal/tls/ package with TLS management and __tls_get_addr
- PT_TLS segment detection in elf/parse.go
- TLS module registration and initialization in loader/loader.go
- TLS relocations in loader/loader.go
- C-callable __tls_get_addr using purego.NewCallback
- Fixed TLS initialization data mapping to account for page alignment
- Comprehensive tests in internal/tls/tls_test.go and dl/dl_test.go

### 7.4 aarch64 Port

The loader is architecture-specific in two places: relocation
types and calling convention bridges. Factor these behind an
interface early (Phase 3) so porting is a matter of adding a
new relocation table and a new trampoline, not restructuring.

**Priority: Medium** — valuable for Linux ARM servers and
container-free deployment there.

---

## Public API Design

Keep the API surface minimal. Users should not need to understand
ELF internals.

```go
package dl

// Open loads a shared library and its dependencies.
func Open(name string, flags ...Flag) (*Library, error)

// Library represents a loaded shared object.
type Library struct { /* unexported */ }

// Sym returns the address of an exported symbol.
func (l *Library) Sym(name string) (uintptr, error)

// Bind resolves a symbol and binds it to a Go function variable.
// fnPtr must be a pointer to a function variable (e.g., *func(float64) float64).
func (l *Library) Bind(name string, fnPtr any) error

// Close decrements the reference count and unloads if zero.
func (l *Library) Close() error
```

---

## Testing Strategy

### Unit Tests

Each phase has its own verification checkpoint (described above).
Additionally:

- Relocation processing: hand-craft minimal ELF binaries in test
  fixtures with known symbol values; assert post-relocation memory
  contents byte-for-byte.
- Hash lookups: test against known symbol names and pre-computed
  SysV/GNU hashes.

### Integration Tests

Maintain a `testdata/` directory with `.c` source files and a
Makefile that compiles them into `.so` files. CI compiles these
fresh on each run. Test cases should cover:

- Simple exported function (add two ints)
- Global variable access
- Library calling its own internal functions
- Library with one dependency
- Library with a deep dependency chain
- Constructor/destructor side effects
- Multiple `Open`/`Close` cycles (refcounting)

### Compatibility Tests

After Phase 5, attempt to load real-world system libraries:

- `libm.so.6` — pure math, no TLS, no complex deps
- `libz.so` — small, widely available, self-contained
- `libsqlite3.so` — substantial, self-contained, exercises many
  relocation types
- `libcurl.so` — deep dependency tree
- `libvulkan.so` — real-world GPU use case matching JangaFX's needs

### CI Configuration

Run all tests with `CGO_ENABLED=0`. If any test requires cgo,
it's a bug in the project, not the test.

---

## Risk Register

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| purego's trampolines are insufficient for complex signatures | Medium | High | Write custom assembly; purego's source is a reference |
| Symbol versioning is needed earlier than Phase 7 | High | Medium | Promote to Phase 5 if loading libc-dependent .so fails |
| IFUNC needed for basic glibc symbols | High | Medium | Implement a minimal IFUNC path in Phase 3 as a stub |
| TLS required by a common library | Low | High | Identify and document which libraries need TLS; avoid them initially |
| Memory leaks from imprecise munmap | Medium | Medium | Track all mappings in a slice; write a debug mode that asserts clean teardown |
| Go runtime interferes with loaded code (signals, memory) | Medium | High | Run integration tests under `-race` and with `GODEBUG=madvdontneed=1` |

---

## Milestones Summary

| Milestone | Phase | Definition of Done | Status |
|---|---|---|---|
| M0: Foundations | 0 | Syscall helpers work in CGO_ENABLED=0; repo structure exists | ✅ COMPLETE |
| M1: Memory Map | 1 | A .so is loaded and a global variable is read from mapped memory | ✅ COMPLETE |
| M2: Symbols | 2 | GNU hash lookup resolves `cos` in `libm.so.6` to a nonzero address | ✅ COMPLETE |
| M3: Relocations | 3 | All GLOB_DAT/JUMP_SLOT/RELATIVE entries process without error | ✅ COMPLETE |
| **M4: First Call** | **4** | **`cos(0) == 1.0` from a CGO_ENABLED=0 binary — announce the project** | ✅ COMPLETE |
| M5: Dependencies | 5 | `libcurl.so` loads with full dependency chain | ✅ COMPLETE |
| M6: Init/Fini | 6 | Constructors and destructors run in correct order | ✅ COMPLETE |
| M7: Versioning | 7.1 | Versioned symbols in libc resolve correctly | ✅ COMPLETE |
| M7.2: IFUNC | 7.2 | IFUNC resolvers execute and return optimized implementations | ✅ COMPLETE |