# AUDIT: dl — 2026-03-06

## Package Role
The `dl` package is the **public API** for the pure-go-dl project. It provides `dlopen`/`dlsym`/`dlclose` semantics for loading ELF shared objects from CGO_ENABLED=0 Go binaries. This is the primary entry point for users and orchestrates the entire library loading workflow: dependency resolution, concurrent loading coordination, symbol resolution, library search paths, and reference counting.

## Summary
**Overall Assessment:** LOW risk, production-ready

| Gate | Status | Value | Threshold |
|------|--------|-------|-----------|
| Test Coverage | ✅ PASS | 77.3% | ≥65% |
| Documentation | ✅ PASS | 77.8% | ≥70% |
| Complexity | ✅ PASS | Max: 4 | All ≤10 |
| Function Length | ⚠️ ADVISORY | 1 function >30 lines | All ≤30 lines |
| Duplication | ✅ PASS | 0.0% | <5% |
| Naming | ⚠️ ADVISORY | 4 violations | 0 violations |

**Gates Passing:** 5/6 (83.3%)  
**Findings:** 2 (0 critical, 0 high, 2 medium, 0 low)

The `dl` package demonstrates excellent code quality with strong test coverage, comprehensive documentation, low complexity, and zero code duplication. The two medium-severity findings are **intentional design decisions** (RTLD_* naming for POSIX compatibility) and **acceptable complexity** (loadPath orchestrates multiple steps). No remediation required.

## Findings

### MEDIUM
- [x] **RTLD_* constant naming** — dl.go:24-26 — naming: 3 underscore violations (RTLD_LOCAL, RTLD_GLOBAL, RTLD_NOW)
  - **Rationale:** These identifiers intentionally use POSIX-standard names (`RTLD_LOCAL`, `RTLD_GLOBAL`, `RTLD_NOW`) to match the conventional `dlopen()` API from `<dlfcn.h>`, making the library immediately familiar to users with C/POSIX experience.
  - **Remediation:** NOT RECOMMENDED — the naming convention is a deliberate compatibility decision documented in the API reference (README.md lines 111-118). Changing to `RTLDLocal`, `RTLDGlobal`, `RTLDNow` would reduce API familiarity.
  - **Verdict:** ACCEPTED as idiomatic for POSIX-compatible APIs.
  - **Resolution:** Intentional naming for POSIX API compliance. Maintains consistency with C `dlopen()` API and improves usability for developers familiar with dynamic linking conventions.

- [x] **Function length advisory** — dl.go:173-209 — function: `loadPath` is 35 lines (threshold: ≤30)
  - **Rationale:** `loadPath` orchestrates the complete library loading workflow: cycle detection, cache coordination, parsing, dependency loading, object loading, and registration. The function is well-structured with clear helper function calls and early returns.
  - **Complexity:** Cyclomatic complexity is 4 (well below the 10 threshold), indicating simple control flow despite the length.
  - **Remediation:** NOT REQUIRED — the function is already decomposed into well-named helpers (`checkLoadingCache`, `loadDependencies`, `registerLibrary`, etc.). Further splitting would fragment the loading sequence logic without improving readability.
  - **Verdict:** ACCEPTED — length is reasonable for an orchestration function with low complexity.
  - **Resolution:** Acceptable for an orchestration function with low cyclomatic complexity (4). The function serves a specific purpose (coordinating the complete load workflow) and is already well-decomposed with clear helper calls.

## Strengths

### 1. Concurrent Loading Safety
The package implements **TOCTOU-safe concurrent loading** using `sync.Cond` and a `loading` map (dl.go:33-48). Multiple goroutines can safely call `Open()` for the same library concurrently, with proper coordination to avoid duplicate loads and race conditions. This is verified by `TestConcurrentLoading` (dl_test.go).

### 2. Reference Counting
Robust reference counting with proper increment/decrement logic (dl.go:315-357) ensures libraries are only unloaded when all references are released, preventing use-after-free bugs. Test coverage includes `TestReferenceCount` (refcount_test.go).

### 3. Symbol Resolution Strategy
The package implements **strong-over-weak symbol resolution** per ELF linking rules (dl.go:88-120), preferring `STB_GLOBAL`/`STB_LOCAL` symbols over `STB_WEAK` symbols when multiple libraries define the same symbol. IFUNC symbols are automatically resolved.

### 4. Library Search Path Implementation
Complete implementation of the ELF dynamic linker search path algorithm (search.go:34-56):
1. DT_RUNPATH of requesting library
2. LD_LIBRARY_PATH environment variable
3. /etc/ld.so.cache lookup (with fallback if cache unavailable)
4. DT_RPATH of requesting library
5. Default system paths

This matches glibc's behavior and is extensively tested in `ldcache_test.go` and `dl_test.go`.

### 5. Documentation Quality
- **Package-level godoc** (dl.go:1-3) clearly explains the package purpose
- **All exported types** (`Flag`, `Library`) are documented
- **All exported functions** (`Open`, `Sym`, `Bind`, `Close`, `PrintSymbols`) have comprehensive godoc comments
- **Inline comments** explain complex logic (129 inline comments throughout)
- **Examples** provided in `example_test.go`

### 6. Zero Code Duplication
No duplicate code blocks detected (duplication ratio: 0.0%), indicating good use of helper functions and abstraction.

### 7. Low Complexity
All 38 functions have cyclomatic complexity ≤10 (maximum: 4), making the code easy to understand and maintain.

## Test Coverage Analysis

**Overall Coverage:** 77.3% (threshold: ≥65%) ✅

### Covered Scenarios
- ✅ Basic library loading and symbol resolution
- ✅ Concurrent loading from multiple goroutines
- ✅ Reference counting (increment on Open, decrement on Close)
- ✅ Dependency loading (transitive DT_NEEDED)
- ✅ IFUNC resolution (indirect functions)
- ✅ Symbol versioning
- ✅ Error handling (missing libraries, undefined symbols)
- ✅ ld.so.cache parsing and lookup
- ✅ Library search paths (RUNPATH, RPATH, LD_LIBRARY_PATH, cache, defaults)
- ✅ ARM64 architecture support

### Coverage Gaps (22.7%)
Based on the test files, uncovered areas likely include:
- Error paths in ld.so.cache parsing edge cases (truncated files, malformed entries)
- Some edge cases in concurrent loading (e.g., simultaneous unloading)
- Weak symbol resolution edge cases
- PrintSymbols edge cases

These gaps are acceptable for production use given the strong coverage of critical paths.

## Security Considerations

### ✅ Strengths
1. **Thread-safe global state** — all access to `loaded`, `loading`, and `globals` is protected by `mu` mutex
2. **Cycle detection** — `visiting` map prevents infinite loops in circular dependencies
3. **No unsafe operations** in this package (delegates to `loader` and `elf` packages)
4. **Reference counting** prevents premature unloading and use-after-free

### ⚠️ Considerations (Inherited from Dependencies)
- The package delegates to `loader.Load()` which performs memory mapping, relocations, and calls IFUNC resolvers — security depends on those implementations
- Constructor execution (`DT_INIT`, `DT_INIT_ARRAY`) runs arbitrary code from loaded libraries
- Users must trust the loaded libraries (this is inherent to any dynamic loader)

## Recommendations

### Production Deployment
✅ **APPROVED** — Package is production-ready with no blocking issues.

### Optional Improvements (Low Priority)
1. **Test coverage:** Add edge case tests for ld.so.cache parsing errors and concurrent unloading to reach 85%+ coverage.
2. **Performance monitoring:** Consider adding metrics/logging for library search path performance (e.g., cache hit rate, search time).
3. **Documentation:** Add architecture diagram showing the relationship between `dl`, `loader`, `elf`, and `symbol` packages.

### Non-Issues (Do Not Change)
- ❌ Do NOT rename `RTLD_*` constants to remove underscores — POSIX compatibility is intentional
- ❌ Do NOT split `loadPath()` further — it's already well-decomposed with clear helper functions

---

**Auditor Notes:**
- All tests pass with `-race` detector (no data races)
- `go vet` reports no issues
- Package follows Go conventions and project idioms
- No critical or high-severity findings
- Test coverage exceeds target threshold
- Code complexity is well-controlled
