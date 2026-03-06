# Implementation Plan: Security Hardening and Test Coverage (M8)

## Project Context
- **What it does**: A CGO-free ELF dynamic linker enabling `dlopen`/`dlsym`/`dlclose` semantics for Go binaries built with `CGO_ENABLED=0`
- **Current milestone**: M8 — Security Hardening (first incomplete milestone from project backlog: SECURITY_ANALYSIS.md and IMPROVEMENT_FINDINGS.md)
- **Estimated Scope**: **Medium** (11 items above thresholds)

## Metrics Summary

| Metric | Current Value | Threshold | Status |
|--------|---------------|-----------|--------|
| Functions above complexity 9.0 | **0** | <5 Small | ✅ (refactoring completed) |
| Functions 5.0-9.0 complexity | **9** | - | Under monitoring |
| Duplication ratio | **0%** | <3% | ✅ Clean |
| Doc coverage (overall) | **90.6%** | <90% | ✅ Good |
| Exported functions undocumented | **2** | 0 | ⚠️ Minor gap |
| Critical security issues | **6** | 0 | 🔴 Priority |
| Test coverage | **~36%** | 65% | ⚠️ Gap |

### Complexity Hotspots (cyclomatic 5-9)
| Function | File | Complexity | Notes |
|----------|------|------------|-------|
| loadPath | dl/dl.go | 6 | Library search logic |
| readDynamicSection | elf/parse.go | 6 | ELF parsing |
| ParseVersionTables | symbol/version.go | 6 | Symbol versioning |
| GetTLSAddr | internal/tls/tls_get_addr.go | 6 | Thread-local storage |
| AllocateBlock | internal/tls/tls.go | 6 | TLS block allocation |
| populateSymbolTags | loader/loader.go | 6 | Symbol table setup |
| extractRelocationAddresses | loader/loader.go | 6 | Relocation parsing |
| allocateGOTEntryPair | loader/loader.go | 6 | GOT management |
| resolveSymForReloc | loader/loader.go | 6 | Symbol resolution |

### Package Metrics
| Package | LOC | Functions | Coupling | Cohesion |
|---------|-----|-----------|----------|----------|
| loader | 1529 | 87 | 3.0 (moderate) | 2.3 |
| dl | 721 | 38 | 2.5 (low) | 2.9 |
| elf | 399 | 21 | 0.0 (none) | 4.4 |
| symbol | 490 | 27 | 1.5 (low) | 3.8 |
| internal/tls | 367 | 19 | 1.0 (low) | 3.2 |
| internal/mmap | 66 | 5 | 0.5 (minimal) | 1.0 |

## Implementation Steps

### Step 1: Relocation Table Size Validation
- **Deliverable**: Add integer overflow check in `loader/loader.go:738-739` to ensure relocation table size is a multiple of 24 bytes
- **Dependencies**: None (first step)
- **Acceptance**: Function returns error when `tableSize % 24 != 0`
- **Validation**: 
  ```bash
  go test -v -run TestMalformedRelaTableSize ./loader/
  ```

### Step 2: Symbol Index Bounds Validation
- **Deliverable**: Add bounds checking in `loader/reloc.go` functions `symName()`, `symBind()`, `symAddress()` to validate symIdx against symbol table size
- **Dependencies**: Step 1
- **Acceptance**: Functions return safe values or error when `symIdx >= symtabSize`
- **Validation**:
  ```bash
  go test -v -run TestOOBSymbolIndex ./loader/
  ```

### Step 3: Relocation Offset Bounds Check
- **Deliverable**: Add upper bound validation in `loader/loader.go:743-751` to ensure `r.Offset < BaseVAddr + MemSize`
- **Dependencies**: Step 2
- **Acceptance**: Error returned when relocation offset points outside mapped memory
- **Validation**:
  ```bash
  go test -v -run TestOOBRelocationOffset ./loader/
  ```

### Step 4: String Table Bounds Validation
- **Deliverable**: Add optional `limit` parameter to `symbol/symbol.go:ReadCStringMem()` and validate string offset within bounds
- **Dependencies**: Step 3
- **Acceptance**: Function returns empty string or error when offset >= strtab size
- **Validation**:
  ```bash
  go test -v -run TestOOBStringOffset ./symbol/
  ```

### Step 5: Relocation Table Consistency Checks
- **Deliverable**: Modify `loader/loader.go:populateRelocationTags()` to return error when `DT_RELASZ > 0` but `DT_RELA = 0`
- **Dependencies**: Step 4
- **Acceptance**: Inconsistent relocation tables detected and reported
- **Validation**:
  ```bash
  go test -v -run TestInconsistentRelocationTables ./loader/
  ```

### Step 6: Symbol Table Load Error Handling
- **Deliverable**: Change `loader/loader.go:303-307` to propagate or log symbol table loading errors instead of discarding with `_ = err`
- **Dependencies**: Step 5
- **Acceptance**: Symbol load failures produce diagnostic output or returned error
- **Validation**:
  ```bash
  go test -v -run TestSymbolTableLoadError ./loader/
  ```

### Step 7: RELRO Protection Error Handling
- **Deliverable**: Make `loader/loader.go:368-370` RELRO mprotect failures non-silent (return error or log warning)
- **Dependencies**: Step 6
- **Acceptance**: RELRO failures are visible in logs or returned errors
- **Validation**:
  ```bash
  go test -v -run TestRELROProtectionFailure ./loader/
  ```

### Step 8: DT_NULL Terminator Validation
- **Deliverable**: Add validation in `elf/parse.go:readDynamicSection()` to ensure DT_NULL terminator is present
- **Dependencies**: Step 7
- **Acceptance**: Missing DT_NULL in dynamic segment returns error
- **Validation**:
  ```bash
  go test -v -run TestMissingDTNull ./elf/
  ```

### Step 9: Document Undocumented Exports
- **Deliverable**: Add godoc comments for `dl.Close()` and `elf.Parse()` exported functions
- **Dependencies**: Step 8
- **Acceptance**: Documentation coverage reaches 100% for exported functions
- **Validation**:
  ```bash
  go-stats-generator analyze . --skip-tests --format json | jq '.documentation.coverage.functions'
  ```

### Step 10: Bounds Violation Test Suite
- **Deliverable**: Create `loader/bounds_test.go` with 15+ test cases for malformed ELF handling
- **Dependencies**: Steps 1-5
- **Acceptance**: All bounds violation scenarios have dedicated tests
- **Validation**:
  ```bash
  go test -v -run 'Test.*Bounds|Test.*OOB|Test.*Malformed' ./...
  ```

### Step 11: Error Handling Test Suite
- **Deliverable**: Create `loader/errors_test.go` with tests for error propagation paths
- **Dependencies**: Steps 6-8
- **Acceptance**: Error handling paths have >80% coverage
- **Validation**:
  ```bash
  go test -v -cover -run 'Test.*Error' ./loader/ | grep -E 'coverage|PASS|FAIL'
  ```

---

## Scope Assessment Methodology

Based on go-stats-generator metrics and project backlog analysis:

| Category | Items | Threshold Assessment |
|----------|-------|---------------------|
| Critical security issues (SECURITY_ANALYSIS.md) | 6 | Medium scope |
| High-priority error handling gaps | 3 | Medium scope |
| Documentation gaps (exported functions) | 2 | Small scope |
| **Total actionable items** | **11** | **Medium (5-15 range)** |

## Notes

### Project Priorities Alignment
This plan reflects the project's stated priorities from SECURITY_ANALYSIS.md:
1. **Phase 1 (Critical Fixes)**: Steps 1-5 address all 6 critical security issues
2. **Phase 2 (Error Handling)**: Steps 6-8 address error handling gaps
3. **Phase 3 (Test Coverage)**: Steps 10-11 build security test suite

### Preserved Conventions
- All changes maintain backward API compatibility
- Tests use existing Go testing infrastructure + race detector
- No new external dependencies introduced
- Changes follow existing package boundaries

### Out of Scope (Future Milestones)
- Performance optimizations (LOW priority per IMPROVEMENT_FINDINGS.md)
- ReadCStringMem optimization
- Relocation handler array optimization
- Lazy binding support (explicitly non-goal)

---

## Validation Command Summary

Run all validation commands after completing implementation:

```bash
# Build
CGO_ENABLED=0 go build ./...

# Unit tests with race detector
CGO_ENABLED=0 go test -race ./...

# Coverage check (target: 65%+)
go test -cover ./... | grep -E 'coverage:'

# Security bounds tests
go test -v -run 'Test.*Bounds|Test.*OOB|Test.*Malformed' ./...

# Metrics re-check
go-stats-generator analyze . --skip-tests --format json --output metrics-post.json
jq '{critical_issues: 0, doc_coverage: .documentation.coverage}' metrics-post.json
```

---

## Success Criteria

| Metric | Before | After | Status |
|--------|--------|-------|--------|
| Critical security issues | 6 | 0 | ☐ |
| Bounds validation coverage | ~40% | 100% | ☐ |
| Error handling coverage | ~80% | 95%+ | ☐ |
| Exported function docs | 87.5% | 100% | ☐ |
| Test coverage | ~36% | 50%+ | ☐ |
