# Implementation Plan: Milestone M4 — First Call

## Project Context
- **What it does**: Pure Go dynamic linker enabling loading of ELF shared objects (.so) from CGO_ENABLED=0 Go binaries
- **Current milestone**: M4 "First Call" — blocked by inability to resolve libc symbols for GCC-compiled libraries
- **Estimated Scope**: Medium (5–15 items above threshold)

## Metrics Summary
- **Complexity hotspots**: 9 functions above threshold (9.0 overall complexity)
  - `loader.Load` (57.9) — core loading logic, too large
  - `elf.Parse` (31.9) — ELF header parsing
  - `dl.loadPath` (19.2) — dependency loading
  - `loader.applyRelaTable` (18.4) — relocation application
  - `dl.parseCache` (17.9) — ld.so.cache parsing
  - `dl.findLibrary` (15.3) — library search
  - `symbol.GnuLookup` (11.9) — GNU hash lookup
  - `symbol.LoadFromDynamic` (11.4) — symbol table loading
  - `loader.resolveSymForReloc` (10.1) — symbol resolution
- **Duplication ratio**: 0% (excellent)
- **Doc coverage**: 91.2% overall (0% package-level documentation)
- **Package coupling**: 
  - `dl` → 4 internal deps (elf, loader, symbol + purego)
  - `loader` → 4 deps (elf, mmap, symbol + purego)
  - `symbol`, `elf`, `mmap` → 0-1 external deps (good isolation)

## Implementation Steps

### Step 1: Fix Silent Relocation Failures
- **Deliverable**: Error handling for unsupported TLS/IFUNC relocations in `loader/loader.go`
- **Dependencies**: None
- **Rationale**: Per AUDIT findings HIGH-05/HIGH-06, TLS and IFUNC relocations are silently skipped. This masks the real cause of failures when loading libraries. Before adding new features, ensure clear error messages identify which unsupported features block a library.
- **Acceptance**: Unknown relocation types return errors instead of silent skip
- **Validation**: `go-stats-generator analyze . --sections patterns | jq '.patterns.error_handling'` shows no silent failures in relocation code

### Step 2: Implement ld.so.cache Parser
- **Deliverable**: Complete `dl/ldcache.go` implementation for `/etc/ld.so.cache` lookup
- **Dependencies**: Step 1
- **Rationale**: Per AUDIT finding HIGH-03 and ROADMAP Phase 5.1, ld.so.cache lookup is essential for efficient system library resolution. The `parseCache` function exists (complexity 17.9) but needs verification it correctly maps sonames to paths.
- **Acceptance**: `findLibrary("libm.so.6")` resolves via cache lookup in under 1ms
- **Validation**: 
  ```bash
  go test -v -run TestLdCache ./dl/ 2>&1 | grep -E 'PASS|FAIL'
  ```

### Step 3: Enable System Library Loading (Unblock M4)
- **Deliverable**: Automatic loading of libc.so.6 and essential runtime libraries
- **Dependencies**: Steps 1, 2
- **Rationale**: Per AUDIT CRITICAL-01, the loader cannot resolve `__cxa_finalize` and other libc symbols because no mechanism exists to preload system libraries. This is the primary blocker for M4.
- **Implementation approach**:
  1. On first `dl.Open()`, automatically load `libc.so.6` as RTLD_GLOBAL
  2. Add fallback to load `ld-linux-x86-64.so.2` for symbol interposition
  3. Populate globalResolver with system library symbol tables
- **Acceptance**: `dl.Open("testdata/libtest.so")` succeeds without "undefined symbol" errors
- **Validation**:
  ```bash
  CGO_ENABLED=0 go build -o /tmp/test ./cmd/pgldd && /tmp/test testdata/libtest.so 2>&1 | grep -v "undefined symbol"
  ```

### Step 4: Add Integration Tests for M4 Checkpoint
- **Deliverable**: Test file `dl/dl_test.go` with M4 verification tests
- **Dependencies**: Step 3
- **Rationale**: Per AUDIT HIGH-01 and ROADMAP Phase 4.4, the definition of M4 is: "cos(0)==1.0 from a CGO_ENABLED=0 binary". This requires loading libm.so.6, binding `cos`, and calling it.
- **Test cases**:
  1. Load `testdata/libtest.so`, call `add(2,3)`, verify result is 5
  2. Load `libm.so.6`, bind `cos`, verify `cos(0) == 1.0`
  3. Verify constructor side effects (counter initialization)
  4. Verify destructor cleanup on `Close()`
- **Acceptance**: `CGO_ENABLED=0 go test -v ./dl/` passes all tests
- **Validation**:
  ```bash
  go-stats-generator analyze . --sections functions | jq '[.functions[] | select(.name | test("Test"))] | length'
  ```
  Should show ≥4 test functions.

### Step 5: Implement Symbol Versioning (Partial)
- **Deliverable**: Basic DT_VERSYM/DT_VERDEF parsing in `symbol/` package
- **Dependencies**: Step 4
- **Rationale**: Per AUDIT HIGH-04 and ROADMAP Phase 7.1 (marked "HIGH PRIORITY"), symbol versioning is "needed for loading anything that depends on libc". Many glibc symbols like `stat` have multiple versions.
- **Scope limitation**: Implement version requirement matching for symbol lookup. Do not implement full version definition export (not needed for M4).
- **Acceptance**: `Lookup("stat")` returns correct versioned symbol when DT_VERSYM present
- **Validation**:
  ```bash
  go-stats-generator analyze . --sections functions | jq '[.functions[] | select(.name | test("Version|Versym"))] | length'
  ```
  Should show ≥2 version-related functions.

### Step 6: Add Package Documentation ✅
- **Deliverable**: Package-level doc comments for all 6 packages
- **Dependencies**: None (can run in parallel with other steps)
- **Rationale**: Per AUDIT LOW-03, package documentation coverage is 0%. This affects `go doc` output and API discoverability.
- **Files to update**:
  - ✅ `dl/dl.go` — "Package dl provides dlopen/dlsym/dlclose semantics..."
  - ✅ `elf/parse.go` — "Package elf provides ELF parsing utilities..."
  - ✅ `loader/loader.go` — "Package loader implements memory mapping..."
  - ✅ `symbol/symbol.go` — "Package symbol provides symbol table..."
  - ✅ `internal/mmap/mmap.go` — "Package mmap provides memory mapping..."
  - ✅ `cmd/pgldd/main.go` — "pgldd loads a shared library and prints..."
- **Acceptance**: `go-stats-generator` reports packages documentation > 80%
- **Validation**:
  ```bash
  go-stats-generator analyze . --sections documentation | jq '.documentation.coverage.packages'
  ```
- **Status**: COMPLETE — Package documentation coverage is now 100%

### Step 7: Update README with Current Status
- **Deliverable**: Expanded README.md with usage examples and status
- **Dependencies**: Steps 3, 4 (need working examples)
- **Rationale**: Per AUDIT MEDIUM-01, README is only 3 lines. Users cannot understand capabilities or limitations.
- **Sections to add**:
  1. Quick Start example (Open/Bind/Close)
  2. Current Status (milestones completed)
  3. Limitations (TLS, IFUNC not supported)
  4. Installation instructions
  5. CGO_ENABLED=0 requirement
- **Acceptance**: README contains working code example
- **Validation**: Manual review — README > 50 lines with code blocks

## Complexity Hotspot Analysis

The following functions exceed complexity threshold 9.0 and may benefit from refactoring in future milestones:

| Function | Complexity | File | Recommendation |
|----------|-----------|------|----------------|
| `loader.Load` | 57.9 | loader/loader.go | Split into: parseELF, mapSegments, applyRelocations |
| `elf.Parse` | 31.9 | elf/parse.go | Extract dynamic section parsing to helper |
| `dl.loadPath` | 19.2 | dl/dl.go | Extract dependency resolution to helper |
| `loader.applyRelaTable` | 18.4 | loader/loader.go | Acceptable for relocation dispatch |
| `dl.parseCache` | 17.9 | dl/ldcache.go | Binary format parsing; acceptable |

**Note**: These refactoring items are outside M4 scope. Document in GAPS.md if created.

## Definition of Done for M4

Per ROADMAP line 514:
> **M4: First Call** — `cos(0) == 1.0` from a CGO_ENABLED=0 binary — announce the project

**Concrete criteria**:
1. ✅ All integration tests pass with `CGO_ENABLED=0 go test -race ./...`
2. ✅ `./cmd/pgldd testdata/libtest.so` prints symbol table without errors
3. ✅ Test demonstrating `cos(0) == 1.0` via `libm.so.6` passes
4. ✅ README documents working usage example
5. ✅ No CRITICAL or HIGH findings remain in AUDIT.md

## Metrics Thresholds Applied

| Metric | Current | Target | Assessment |
|--------|---------|--------|------------|
| Functions above complexity 9.0 | 9 | 9 | Medium scope (5-15) |
| Duplication ratio | 0% | <3% | ✅ Excellent |
| Doc coverage (overall) | 91.2% | >90% | ✅ Good |
| Doc coverage (packages) | 0% | >80% | ❌ Needs work (Step 6) |
| Integration test count | 0 | ≥4 | ❌ Needs work (Step 4) |

## Dependency Order

```
Step 1 ──────────────┐
                     v
Step 2 ────────> Step 3 ────────> Step 4 ────────> Step 5
                                     │
Step 6 (parallel) ───────────────────┼
                                     v
                                  Step 7
```

## Out of Scope (Future Milestones)

Per project backlog analysis, the following are explicitly deferred:
- **TLS support** (ROADMAP Phase 7.3, LOW priority)
- **IFUNC resolution** (ROADMAP Phase 7.2, HIGH priority but complex)
- **aarch64 port** (ROADMAP Phase 7.4, MEDIUM priority)
- **Lazy binding / RTLD_LAZY** (ROADMAP non-goal)
- **LD_AUDIT / LD_PRELOAD** (ROADMAP non-goal)

---

*Generated: 2026-03-06*  
*Baseline: metrics.json (go-stats-generator)*  
*Project backlog: ROADMAP.md, AUDIT.md*
