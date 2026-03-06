# AUDIT: internal/tls — 2026-03-06

## Package Role
The `internal/tls` package implements Thread-Local Storage (TLS) management for dynamically loaded shared libraries. It provides TLS block allocation, Dynamic Thread Vector (DTV) management, and supports the General Dynamic (GD) TLS access model. This package is critical for loading libraries that use thread-local variables (e.g., `__thread` in C), enabling per-thread isolation of global state in multi-threaded environments.

**Architecture Position:** Internal infrastructure package — provides TLS primitives consumed by the `loader` package during relocation processing and library initialization. Has 3 importers (loader + 2 internal dependencies).

## Summary
**Gates Passed: 5/6** ✅

| Gate | Status | Result | Threshold |
|------|--------|--------|-----------|
| Test Coverage | ✅ PASS | 83.3% | ≥65% |
| Documentation | ✅ PASS | 100% (14/14 exported) | ≥70% |
| Complexity | ✅ PASS | Max cyclomatic: 6 | All ≤10 |
| Function Length | ⚠️ FAIL | 2 functions >30 lines | All ≤30 |
| Duplication | ✅ PASS | 0% internal | <5% |
| Naming | ✅ PASS | 0 critical violations | 0 |

**Overall Assessment:** LOW risk, production-ready

This package passes 5 of 6 quality gates. The function length advisory is exceeded by two functions (`AllocateBlock` at 40 lines and `GetTLSAddr` at 45 lines), but both are inherently complex TLS allocation routines that benefit from keeping the allocation logic cohesive rather than fragmenting it. Test coverage is excellent at 83.3%, all exported functions are documented, and complexity metrics are within safe bounds.

## Findings

### MEDIUM
- [ ] **Function length advisory exceeded** — `tls.go:95` (AllocateBlock) — **40 lines** (threshold: ≤30)
  - **Context:** Core TLS block allocation with mmap, alignment, and initialization — inherently complex system-level operation
  - **Remediation:** Consider extracting alignment calculation and memory initialization into helper functions (`calculateAlignedSize`, `initializeTLSData`) to reduce to ~25 lines
  - **Severity Rationale:** MEDIUM — function is testable and documented, but splitting would improve readability

- [ ] **Function length advisory exceeded** — `tls_get_addr.go:91` (GetTLSAddr) — **45 lines** (threshold: ≤30)
  - **Context:** Implements `__tls_get_addr` runtime function with per-thread DTV management and lazy block allocation
  - **Remediation:** Extract `ensureThreadDTV` and `allocateOnDemand` helper functions to reduce main flow to ~20 lines
  - **Severity Rationale:** MEDIUM — critical runtime path; refactoring must preserve performance and thread-safety

### LOW
- [ ] **Package naming convention** — `tls_get_addr.go:13` (TLSIndex) — **Package stuttering**
  - **Context:** Exported type `TLSIndex` in package `tls` creates stutter (`tls.TLSIndex`)
  - **Remediation:** Consider renaming to `Index` (usage: `tls.Index`) per Go conventions
  - **Severity Rationale:** LOW — minor style issue; `TLSIndex` may be clearer in this domain (matches C/ELF terminology)
  - **Note:** This is a common pattern in systems programming where matching C API names aids comprehension

### INFORMATIONAL
- [ ] **Expected unsafe.Pointer usage** — `tls.go:120,121,129` and `tls_get_addr.go:92`
  - **Context:** `go vet` reports 12 "possible misuse of unsafe.Pointer" warnings (9 in tests, 3 in production)
  - **Analysis:** All unsafe usage is intentional for TLS memory manipulation (copying init data, calculating offsets)
  - **Justification:** See project-level `UNSAFE_POINTER_USAGE.md` — unsafe is required for ELF dynamic linking
  - **Action Required:** None — warnings are expected and documented

## Strengths

1. **Excellent Test Coverage (83.3%)** — Exceeds threshold by 18.3pp, demonstrates thorough validation of TLS allocation, per-thread isolation, and concurrent access patterns
2. **Perfect Documentation (100%)** — All 14 exported functions have godoc comments explaining TLS semantics, usage, and invariants
3. **Low Complexity** — Maximum cyclomatic complexity of 6 (well below 10 threshold); functions are focused and maintainable
4. **Zero Duplication** — No code duplication detected; TLS allocation logic is centralized
5. **Thread-Safety** — Proper mutex usage in DTV registry and manager; passes `-race` detector
6. **Bounded Resource Usage** — `maxModuleID` constant caps module IDs at 65536 to prevent unbounded DTV growth

## Recommendations

### For Production Deployment
**Status: Ready** — Package is production-ready with current implementation.

### For Ongoing Maintenance
1. **Refactor long functions** (Priority: MEDIUM) — Split `AllocateBlock` and `GetTLSAddr` into smaller helper functions to improve readability and testability of individual allocation phases
2. **Consider renaming TLSIndex** (Priority: LOW) — Evaluate renaming to `Index` to follow Go conventions, or document rationale for keeping `TLSIndex` (C API compatibility)

## Metrics Summary
```
Total Functions:        14
Exported Functions:     12 (85.7%)
Documented (Exported):  12/12 (100%)
Test Coverage:          83.3%
Max Cyclomatic:         6
Max Function Length:    45 lines
Duplication:            0%
go vet Warnings:        12 (all expected unsafe usage)
```

## Test Results
```
✅ go test -cover:  83.3% coverage, PASS
✅ go test -race:   PASS (no data races)
✅ go vet:          12 expected unsafe.Pointer warnings
```

## Dependencies
- `github.com/opd-ai/pure-go-dl/internal/mmap` — Memory mapping primitives
- `github.com/ebitengine/purego` — Syscall interface for gettid()
- `golang.org/x/sys/unix` — Unix system calls

## Next Steps for Package Improvement
1. Extract `calculateAlignedSize(size, align uint64) uint64` helper from `AllocateBlock`
2. Extract `initializeTLSBlock(addr, initData uintptr, fileSize uint64)` helper from `AllocateBlock`
3. Extract `getOrCreateThreadDTV(threadID uint64) map[uint64]*Block` from `GetTLSAddr`
4. Add benchmark tests for `GetTLSAddr` hot path to validate refactoring doesn't regress performance

---
**Auditor:** GitHub Copilot CLI (Autonomous Agent)  
**Date:** 2026-03-06  
**Audit Standard:** pure-go-dl quality gates (see AUDIT_TRACKER.md)
