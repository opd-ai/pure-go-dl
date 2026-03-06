# Functional Audit Report — pure-go-dl

**Date:** March 6, 2026  
**Project Version:** v0.0.0 (pre-release)  
**Auditor:** GitHub Copilot CLI  
**Baseline Data:** audit-baseline.json (go-stats-generator)

---

## Project Context

**pure-go-dl** is a Pure-Go ELF dynamic linker that claims to enable loading native shared libraries (`.so` files) from statically-linked Go binaries built with `CGO_ENABLED=0`. The project targets x86-64 and ARM64/aarch64 Linux platforms and provides `dlopen`/`dlsym`/`dlclose` semantics without requiring cgo or dynamic linking.

**Target Audience:** Go developers deploying single-binary applications that need to load platform-specific libraries (GPU drivers, system libraries) while avoiding cgo build complexity.

**Claimed Milestones:** M0-M7.4 complete, including foundations, memory mapping, symbols, relocations, dependencies, init/fini, versioning, IFUNC, TLS, and ARM64 support.

---

## Executive Summary

**Overall Health:** MODERATE with 1 CRITICAL issue, 4 HIGH-severity gaps, and several medium/low findings.

### Findings Summary

| Severity | Count | Status |
|----------|-------|--------|
| **CRITICAL** | 1 | Binary requires dynamic linking despite claims |
| **HIGH** | 4 | System library compatibility claims not verified in CI |
| **MEDIUM** | 5 | Test coverage gaps and complexity hotspots |
| **LOW** | 3 | Minor documentation inconsistencies |

### Key Verdict

The project successfully implements the core ELF loading mechanism and works well with custom-compiled test libraries. However, the **fundamental claim that it produces "statically-linked" binaries is FALSE** — binaries built with this library depend on `libdl.so.2`, `libpthread.so.0`, and `libc.so.6` at runtime. This is due to the `purego` dependency, which uses runtime `dlopen`/`dlsym` calls.

Additionally, **system library compatibility claims (libm.so.6, libz.so) are not validated** — these tests are skipped by default with a comment stating they "may crash during init."

---

## Findings

### CRITICAL

- [x] **[CRITICAL-01] Binary requires dynamic linking despite "Pure-Go" and "statically-linked" claims** — Multiple locations in README
  
  **Evidence:**
  ```bash
  $ CGO_ENABLED=0 go build ./cmd/pgldd
  $ file pgldd
  pgldd: ELF 64-bit LSB executable, x86-64, dynamically linked, interpreter /lib64/ld-linux-x86-64.so.2
  
  $ ldd pgldd
  linux-vdso.so.1
  libdl.so.2 => /lib/x86_64-linux-gnu/libdl.so.2
  libpthread.so.0 => /lib/x86_64-linux-gnu/libpthread.so.0
  libc.so.6 => /lib/x86_64-linux-gnu/libc.so.6
  ```
  
  **Root Cause:** The `github.com/ebitengine/purego` dependency (v0.10.0) uses runtime calls to system `dlopen`/`dlsym` via assembly trampolines and fake CGO symbols. This forces the Go linker to produce a dynamically-linked binary even with `CGO_ENABLED=0`.
  
  **Impact:** The core value proposition is undermined. Users expecting a truly statically-linked binary for containerized/embedded deployments will encounter missing library dependencies at runtime. The README's claim of "Deploy single-binary Go applications" is technically incorrect.
  
  **Locations:**
  - README.md:3 — "from statically-linked Go binaries built with `CGO_ENABLED=0`"
  - README.md:7 — "allows Go applications to load and call native C libraries at runtime while remaining **fully statically linked**"
  - README.md:33 — "`CGO_ENABLED=0` (statically-linked Go binary)"
  - README.md:226 — "Verify CGO_ENABLED=0 works ... Should show 'statically linked'"
  - go.mod:5 — `require github.com/ebitengine/purego v0.10.0`
  
  **Recommendation:** Update README to clarify that binaries are "CGO-free" but not "statically linked" — they require libc, libdl, and libpthread at runtime. Add a dedicated section explaining the purego dependency's runtime requirements so users know what to expect in their deployment environments.

---

### HIGH

- [ ] **[HIGH-01] System library compatibility claims not validated by CI** — README.md:172-177
  
  **Evidence:** The README previously claimed "The loader successfully handles: ✅ libm.so.6 (math library), ✅ libz.so (compression), ✅ Most glibc-based system libraries." However, the corresponding tests are **skipped by default** and require `PURE_GO_DL_TEST_SYSTEM_LIBS=1` to run.
  
  From `dl/compat_test.go`:
  ```go
  t.Skip("Skipping libm test - set PURE_GO_DL_TEST_SYSTEM_LIBS=1 to enable")
  ```
  
  **Partial Resolution:** The unverified ✅ checkmarks for libm.so.6 and libz.so have been removed from README. These are now marked ⚠️ to indicate they are not CI-validated.
  
  **Remaining Work:** Fix the underlying glibc IFUNC resolution and init function compatibility issues in the loader so that `TestCompatibility_libm` and `TestCompatibility_libz` pass without the environment flag. Once verified, restore ✅ checkmarks and enable the tests in CI.
  
  **Impact:** Users can no longer be misled by unverified checkmarks. The complete fix requires resolving the loader's compatibility with glibc's init functions.
  
  **Locations:**
  - dl/compat_test.go:48-92 — `TestCompatibility_libm` (skipped pending loader fix)
  - dl/compat_test.go:177-180 — `TestCompatibility_libz` (skipped pending loader fix)
  - dl/compat_test.go:219-222 — `TestCompatibility_libc` (skipped pending loader fix)
  - README.md:194-201 — Library Compatibility section (unverified checkmarks removed)

- [ ] **[HIGH-02] Test coverage for loader package is low (45.7%)** — loader/loader.go
  
  **Evidence:** `go test -coverprofile=coverage.out ./... && go tool cover -func=coverage.out` shows loader package at 45.7% coverage, which is below the 70% threshold for a critical component.
  
  **Impact:** The loader package contains the most complex code (relocation handling, memory mapping, init/fini execution) and is the highest-risk area for bugs. Low test coverage means edge cases in relocation types, library search paths, and error conditions may not be exercised.
  
  **Locations:**
  - loader/loader.go — 933 lines, 45.7% coverage
  - loader/reloc_amd64.go — Relocation handling (untested edge cases)
  - loader/reloc_arm64.go — ARM64 relocation handling (untested edge cases)
  
  **Recommendation:** Add integration tests covering:
  - Libraries with uncommon relocation types (R_X86_64_32S, R_X86_64_PC32, etc.)
  - Error paths (missing symbols, invalid ELF files, circular dependencies)
  - ARM64 relocation scenarios (currently no ARM64-specific tests visible)

- [ ] **[HIGH-03] No verification that pgldd CLI tool works as documented** — README.md:89-96, cmd/pgldd/main.go
  
  **Evidence:** The README provides example usage of `pgldd /lib/x86_64-linux-gnu/libm.so.6` but there are no automated tests verifying that the CLI tool actually runs and produces the expected output.
  
  The `cmd/pgldd` package shows 0.0% coverage:
  ```
  ok  	github.com/opd-ai/pure-go-dl/cmd/pgldd	1.067s	coverage: 0.0% of statements
  ```
  
  **Impact:** Documentation drift — the CLI tool's output format or behavior could change without detection. Users following the README examples may encounter errors.
  
  **Locations:**
  - cmd/pgldd/main.go:17-57 — Main function (no tests)
  - README.md:92-96 — Example usage with no corresponding test
  
  **Recommendation:** Add `cmd/pgldd/main_test.go` with integration tests that invoke the tool on testdata libraries and validate the output format.

- [ ] **[HIGH-04] Reference counting test coverage insufficient** — dl/dl.go:236-260
  
  **Evidence:** The README claims "Reference counting for proper cleanup" but only one test (`TestRefCounting`) validates this behavior. Complex scenarios like:
  - Multiple goroutines loading/closing the same library concurrently
  - Circular dependencies between libraries
  - Close() called after a dependency is already unloaded
  
  are not covered.
  
  **Impact:** Race conditions or use-after-free bugs in reference counting could cause crashes in multi-threaded applications. The `sync.Mutex` and `sync.Cond` usage in dl.go suggests awareness of concurrency issues, but no race detector tests validate thread safety.
  
  **Locations:**
  - dl/dl.go:39-44 — Global state with mutex/cond
  - dl/dl_test.go:98-119 — `TestRefCounting` (only basic scenario)
  
  **Recommendation:** Add stress tests with `go test -race` covering concurrent Open/Close, dependency chains, and global resolver access.

---

### MEDIUM

- [ ] **[MEDIUM-01] Function complexity hotspots exceed best-practice thresholds** — loader/loader.go:213, dl/dl.go:120, dl/ldcache.go:64
  
  **Evidence:** go-stats-generator analysis shows 7 functions with cyclomatic complexity >10 or length >50 lines:
  
  | Function | File:Line | Cyclomatic | Lines | Assessment |
  |----------|-----------|------------|-------|------------|
  | `populateDynamicTags` | loader/loader.go:213 | 17 | 48 | High complexity |
  | `loadPath` | dl/dl.go:120 | 14 | 86 | Long and complex |
  | `parseCache` | dl/ldcache.go:64 | 13 | 84 | Binary parsing, acceptable |
  | `mapSegments` | loader/loader.go:145 | 11 | 62 | Moderate risk |
  | `findLibrary` | dl/search.go:45 | 11 | 49 | Multiple search paths |
  | `LoadFromDynamic` | symbol/symbol.go:113 | 10 | 58 | Symbol table parsing |
  | `GnuLookup` | symbol/gnu_hash.go:31 | 8 | 65 | Hash algorithm, acceptable |
  
  **Impact:** High-complexity functions are harder to maintain, debug, and extend. Future contributors may introduce bugs when modifying these areas.
  
  **Recommendation:** Consider refactoring `loadPath` and `populateDynamicTags` to extract helper functions. Not urgent for current milestone but should be tracked for future work.

- [x] **[MEDIUM-02] README Quick Start example uses libm.so.6 which doesn't work** — README.md:47-63
  
  **Evidence:** Previously, the Quick Start section showed:
  ```go
  lib, err := dl.Open("libm.so.6")  // This will crash per compat_test.go
  var cos func(float64) float64
  lib.Bind("cos", &cos)
  fmt.Println(cos(0)) // Output: 1.0
  ```
  
  Before this fix, `TestCompatibility_libm` required `PURE_GO_DL_TEST_SYSTEM_LIBS=1` and was not validated in CI.
  
  **Impact:** New users would copy-paste this example and encounter crashes, creating a poor first impression.
  
  **Resolution:** Quick Start example replaced with a generic custom-library `add` function example that works with any library compiled with `-fPIC -shared`, directing users to a functional code path.
  
  **Locations:**
  - README.md:47-63 — Quick Start example (fixed)

- [ ] **[MEDIUM-03] ARM64 support claimed but no architecture-specific tests** — README.md:149, loader/reloc_arm64.go
  
  **Evidence:** README line 149 states "✅ M7.4: ARM64 Port — Full aarch64/ARM64 architecture support for Linux" but:
  - No ARM64-specific test libraries in `testdata/`
  - No conditional test execution for ARM64 relocation types
  - The `reloc_arm64.go` file exists (5930 bytes) but coverage data doesn't distinguish between architectures
  
  **Impact:** ARM64 support may be untested and break silently on aarch64 platforms. Users on ARM64 servers will lack confidence in the implementation.
  
  **Locations:**
  - loader/reloc_arm64.go:1 — `//go:build arm64 && linux`
  - README.md:149 — Completion claim
  - testdata/ — No ARM64-specific test fixtures
  
  **Recommendation:** Add ARM64-specific integration tests that verify relocation types like `R_AARCH64_RELATIVE`, `R_AARCH64_GLOB_DAT`, `R_AARCH64_TLS_TPREL`, etc. Consider CI setup for multi-arch testing.

- [ ] **[MEDIUM-04] Symbol versioning test coverage is indirect** — symbol/version.go, dl/compat_test.go:297-321
  
  **Evidence:** The README claims "✅ Symbol versioning support (`DT_VERSYM`, `DT_VERDEF`, `DT_VERNEED`)" but the only test (`TestCompatibility_SymbolVersioning`) simply verifies that loading glibc doesn't crash. It doesn't validate that:
  - Multiple versions of the same symbol (e.g., `stat@GLIBC_2.2.5` vs `stat@GLIBC_2.33`) resolve correctly
  - Version requirements (`DT_VERNEED`) are enforced
  - Unversioned symbols fall back correctly
  
  **Impact:** Subtle versioning bugs could cause incorrect symbol binding, leading to runtime errors or wrong function behavior when loading libraries with multiple symbol versions.
  
  **Locations:**
  - symbol/version.go:53-195 — Version parsing logic (66.7% coverage on `ParseVersionTables`)
  - dl/compat_test.go:297-321 — Indirect test (just loads libc)
  
  **Recommendation:** Create a custom test library with explicitly versioned symbols and verify that `Lookup` and `LookupVersion` select the correct version.

- [ ] **[MEDIUM-05] Weak symbol test only checks "undefined weak", not "defined weak override"** — dl/dl_test.go:136-156
  
  **Evidence:** `TestWeakSymbolsResolveToZero` verifies that undefined weak symbols resolve to address 0, which is correct per ELF spec. However, it doesn't test the scenario where:
  - Library A defines a weak symbol `foo`
  - Library B defines a strong symbol `foo`
  - Library C links against both and should see B's `foo`, not A's
  
  **Impact:** Weak symbol resolution during relocation may not follow correct precedence rules, leading to wrong function calls.
  
  **Locations:**
  - dl/dl_test.go:136-156 — `TestWeakSymbolsResolveToZero`
  - loader/loader.go:775-780 — Weak symbol handling (bind == 2)
  
  **Recommendation:** Add test libraries with overlapping weak/strong symbols and verify symbol resolution precedence.

---

### LOW

- [ ] **[LOW-01] README installation section doesn't mention Go version requirement** — README.md:24-28
  
  **Evidence:** The Installation section says "Requirements: Go 1.24 or later" but `go get github.com/opd-ai/pure-go-dl` is shown without verifying the Go version first.
  
  **Impact:** Users on older Go versions will get cryptic errors. Minor usability issue.
  
  **Locations:**
  - README.md:26 — `go get` command
  - README.md:31 — Requirements listed after installation
  
  **Recommendation:** Reorder to show requirements before installation commands.

- [x] **[LOW-02] Inconsistent terminology: "pure-go" vs "CGO_ENABLED=0"** — README.md (multiple)
  
  **Evidence:** The project name and README used "Pure-Go" in ways implying no runtime dependencies, which was misleading.
  
  **Resolution:** README updated to use "CGO-free" in the description (line 3) and adds an explicit "Important:" notice on line 9 explaining that binaries are **not statically linked** and require `libc.so.6`, `libdl.so.2`, and `libpthread.so.0` at runtime. The "fully statically linked" claim has been removed.
  
  **Locations:**
  - README.md:3 — "CGO-free ELF dynamic linker" (fixed)
  - README.md:9 — Explicit runtime requirements notice (added)

- [ ] **[LOW-03] UNSAFE_POINTER_USAGE.md exists but is not referenced in README** — UNSAFE_POINTER_USAGE.md
  
  **Evidence:** The repository contains `UNSAFE_POINTER_USAGE.md` (presumably documenting unsafe.Pointer usage patterns) but README.md doesn't link to it in the Development or Contributing sections.
  
  **Impact:** Contributors may not be aware of unsafe pointer conventions used in the codebase.
  
  **Locations:**
  - UNSAFE_POINTER_USAGE.md — Exists in repo root
  - README.md:243-250 — Contributing section (no mention)
  
  **Recommendation:** Add link to UNSAFE_POINTER_USAGE.md in README's Development section.

---

## Metrics Snapshot

### Code Statistics
- **Total packages:** 7 (dl, elf, loader, symbol, internal/mmap, internal/tls, cmd/pgldd)
- **Total functions:** 106
- **Total source lines:** ~1,642 (non-test .go files)
- **Average cyclomatic complexity:** 3.71 (excellent)
- **Functions > complexity 10:** 7 (6.6% of total)
- **Functions > 50 lines:** 7 (6.6% of total)

### Documentation Coverage
- **Overall:** 93.7%
- **Packages:** 100% (7/7 have package docs)
- **Functions:** 91.7% (97/106 documented)
- **Exported symbols:** 100% (all have documentation)

### Test Coverage
| Package | Coverage |
|---------|----------|
| dl | 69.3% |
| elf | 81.1% |
| symbol | 78.7% |
| loader | **45.7%** (below target) |
| internal/mmap | 92.9% |
| internal/tls | 84.0% |
| cmd/pgldd | **0.0%** (no tests) |
| **Overall** | **64.6%** |

### Complexity Hotspots (Cyclomatic >10)
1. `populateDynamicTags` — loader/loader.go:213 (complexity 17, 48 lines)
2. `loadPath` — dl/dl.go:120 (complexity 14, 86 lines)
3. `parseCache` — dl/ldcache.go:64 (complexity 13, 84 lines)
4. `mapSegments` — loader/loader.go:145 (complexity 11, 62 lines)
5. `findLibrary` — dl/search.go:45 (complexity 11, 49 lines)

### go vet Results
- **Status:** 45 warnings (all "possible misuse of unsafe.Pointer")
- **Assessment:** Expected for low-level ELF loader; matches UNSAFE_POINTER_USAGE.md scope
- **Action:** No immediate action required; warnings are in symbol parsing and memory access code

---

## Verification Tests Performed

1. ✅ **Build with CGO_ENABLED=0:** Successful but produces dynamically-linked binary (CRITICAL-01)
2. ✅ **Test suite execution:** `go test -race ./...` — All enabled tests pass
3. ✅ **Example tests:** All 4 example tests pass with custom test libraries
4. ⚠️ **System library loading:** Skipped by default (libm, libz, libc tests disabled)
5. ✅ **Custom library loading:** testdata/libtest.so, libreloc.so, libifunc.so, libtls.so all work
6. ✅ **Constructor execution:** libtest.so constructor sets counter=42 (verified)
7. ✅ **IFUNC resolution:** libifunc.so add_ifunc returns correct result
8. ✅ **TLS support:** libtls.so multi-threaded TLS tests pass
9. ⚠️ **ARM64 support:** No arch-specific tests found (MEDIUM-03)
10. ⚠️ **CLI tool:** Not tested (HIGH-03)

---

## Recommendations Summary

### Immediate (Pre-Release Blockers)
1. **Fix CRITICAL-01:** Update README to clarify dynamic linking requirements
2. **Fix HIGH-01:** Either enable system library tests or remove ✅ checkmarks from README
3. **Fix MEDIUM-02:** Replace libm.so.6 Quick Start example with working testdata library

### Near-Term (Next Milestone)
4. **Address HIGH-02:** Increase loader package test coverage to >70%
5. **Address HIGH-03:** Add CLI tool integration tests
6. **Address HIGH-04:** Add concurrent reference counting stress tests
7. **Address MEDIUM-03:** Add ARM64-specific relocation tests

### Long-Term (Technical Debt)
8. **Refactor complexity hotspots** (MEDIUM-01) — Split loadPath and populateDynamicTags
9. **Improve symbol versioning tests** (MEDIUM-04) — Test actual version selection logic
10. **Link UNSAFE_POINTER_USAGE.md from README** (LOW-03) — Add link in the Development section so contributors discover the unsafe pointer conventions

---

## Conclusion

The pure-go-dl project demonstrates **strong engineering fundamentals** with excellent documentation coverage (93.7%), clean code organization, and comprehensive test fixtures. The core ELF loading, relocation, and symbol resolution mechanisms work correctly for custom-compiled libraries.

The README has been updated to accurately reflect that binaries are CGO-free but dynamically linked at runtime. The Quick Start guide now demonstrates a working, reproducible example using test libraries that are validated by CI. README checkmarks for system libraries are only shown for features verified by passing tests.

**Outstanding work:** Before announcing this project, resolve HIGH-01 (fix glibc init-function compatibility so libm/libz load reliably), HIGH-02 (increase loader test coverage), and HIGH-03 (add CLI integration tests).

---

**Audit Completed:** March 6, 2026  
**Baseline:** audit-baseline.json  
**Test Execution:** CGO_ENABLED=0 go test -race ./...  
**Static Analysis:** go vet ./... (45 unsafe.Pointer warnings, expected)
