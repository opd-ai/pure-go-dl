# CI/CD Configuration

This directory contains GitHub Actions workflows for automated testing and building.

## Workflows

### CI (`ci.yml`)

Runs on every push and pull request to `main`/`master` branches.

**Test Job:**
- Tests against Go 1.24 and 1.25
- Builds test libraries from `testdata/`
- Runs all tests with `CGO_ENABLED=0`
- Checks test coverage (minimum 60%)
- Runs `go vet` (unsafe.Pointer warnings are expected, see `UNSAFE_POINTER_USAGE.md`)
- Verifies code formatting with `gofmt`
- Uploads coverage to Codecov

**Build Matrix Job:**
- Builds for Linux amd64 and arm64 architectures
- Verifies cross-compilation works

## Expected Warnings

The CI workflow expects `go vet` warnings about "possible misuse of unsafe.Pointer". These are **documented and safe** - see `UNSAFE_POINTER_USAGE.md` for details.

All warnings are from:
- Working with mmap'd memory addresses
- Reading ELF structures from mapped memory
- Applying relocations

This is inherent to implementing a dynamic linker in pure Go.

## Requirements

- Ubuntu latest (for test library compilation with gcc)
- Go 1.24+ (matches project requirements)
- build-essential, gcc (for compiling test `.so` files)
