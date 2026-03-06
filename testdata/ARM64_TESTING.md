# ARM64 Testing Guide

This directory contains ARM64-specific test resources for validating the pure-go-dl loader on aarch64/ARM64 Linux platforms.

## Test Libraries

### libarm64.so
Exercises core ARM64 relocation types:
- **R_AARCH64_RELATIVE** - Position-independent code relocations
- **R_AARCH64_GLOB_DAT** - Global data access via GOT
- **R_AARCH64_JUMP_SLOT** - PLT function calls
- **R_AARCH64_ABS64** - 64-bit absolute addresses
- **R_AARCH64_CALL26** - Direct function calls (26-bit offset)
- **R_AARCH64_ADR_PREL_PG_HI21** - PC-relative page address (ADRP instruction)
- **R_AARCH64_ADD_ABS_LO12_NC** - Page offset (ADD instruction)
- **R_AARCH64_LDST64_ABS_LO12_NC** - Load/store with 12-bit offset
- **R_AARCH64_IRELATIVE** - Indirect function resolution (IFUNC)

Features tested:
- Global variable access
- Static local array access
- Internal function calls
- Function pointer arrays
- Constructor execution (DT_INIT_ARRAY)
- Weak symbols
- IFUNC (runtime function selection)

### libarm64_tls.so
Exercises ARM64 Thread-Local Storage (TLS) relocations:
- **R_AARCH64_TLS_TPREL64** - Thread-local offset (64-bit)
- **R_AARCH64_TLSLE_ADD_TPREL_HI12** - TLS local-exec high 12 bits
- **R_AARCH64_TLSLE_ADD_TPREL_LO12_NC** - TLS local-exec low 12 bits
- **R_AARCH64_TLSLE_LDST64_TPREL_LO12_NC** - TLS load/store with offset
- **R_AARCH64_TLSIE_ADR_GOTTPREL_PAGE21** - TLS initial-exec page address
- **R_AARCH64_TLSIE_LD64_GOTTPREL_LO12_NC** - TLS initial-exec offset

Features tested:
- Thread-local variables
- Thread-local arrays
- Thread-local structs
- TLS access via pointers
- TLS in constructors
- Multi-threaded TLS isolation

## Building Test Libraries

On an ARM64/aarch64 Linux system:

```bash
# Build all ARM64-specific libraries
make -C testdata arm64

# This creates:
# - libarm64.so
# - libarm64_tls.so
```

Requirements:
- GCC compiler for aarch64
- glibc with TLS support
- pthread library

## Running Tests

### On ARM64 Hardware

```bash
# Run all tests (including ARM64-specific)
CGO_ENABLED=0 go test -v ./...

# Run only loader ARM64 relocation tests
GOARCH=arm64 go test -v ./loader -run TestARM64

# Run only dl ARM64 integration tests
GOARCH=arm64 go test -v ./dl -run TestARM64
```

### On x86_64 (Cross-Compilation Verification)

ARM64-specific test files use `//go:build arm64 && linux` constraints, so they won't run on x86_64. However, you can verify that ARM64 code compiles:

```bash
# Verify ARM64 code compiles
GOARCH=arm64 go build ./loader
GOARCH=arm64 go build ./dl

# Verify test files compile (but don't run)
GOARCH=arm64 go test -c ./loader
GOARCH=arm64 go test -c ./dl
```

## CI/CD Integration

### GitHub Actions Example

```yaml
name: ARM64 Tests

on: [push, pull_request]

jobs:
  test-arm64:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      
      - name: Set up QEMU
        uses: docker/setup-qemu-action@v2
        with:
          platforms: arm64
      
      - name: Build ARM64 test libraries
        run: |
          docker run --rm -v $PWD:/workspace -w /workspace \
            arm64v8/gcc:latest \
            make -C testdata arm64
      
      - name: Run ARM64 tests
        run: |
          docker run --rm -v $PWD:/workspace -w /workspace \
            arm64v8/golang:1.24 \
            go test -v ./...
```

### Native ARM64 Runner

If you have access to native ARM64 CI runners (AWS Graviton, Ampere, Apple Silicon):

```yaml
jobs:
  test-arm64-native:
    runs-on: [self-hosted, linux, arm64]
    steps:
      - uses: actions/checkout@v3
      
      - name: Build test libraries
        run: make -C testdata arm64
      
      - name: Run tests with race detector
        run: CGO_ENABLED=0 go test -race -v ./...
```

## Test Coverage

The ARM64 tests verify:

1. **Relocation Types**
   - ✅ R_AARCH64_RELATIVE
   - ✅ R_AARCH64_GLOB_DAT
   - ✅ R_AARCH64_JUMP_SLOT
   - ✅ R_AARCH64_ABS64
   - ✅ R_AARCH64_ABS32
   - ✅ R_AARCH64_CALL26
   - ✅ R_AARCH64_ADR_PREL_PG_HI21
   - ✅ R_AARCH64_ADD_ABS_LO12_NC
   - ✅ R_AARCH64_LDST64_ABS_LO12_NC
   - ✅ R_AARCH64_IRELATIVE

2. **TLS Relocations**
   - ✅ R_AARCH64_TLS_TPREL64
   - ✅ R_AARCH64_TLSLE_ADD_TPREL_HI12
   - ✅ R_AARCH64_TLSLE_ADD_TPREL_LO12_NC
   - ✅ R_AARCH64_TLSLE_LDST64_TPREL_LO12_NC
   - ✅ R_AARCH64_TLSIE_ADR_GOTTPREL_PAGE21
   - ✅ R_AARCH64_TLSIE_LD64_GOTTPREL_LO12_NC

3. **Features**
   - ✅ Library loading and unloading
   - ✅ Symbol resolution
   - ✅ Function binding and calls
   - ✅ Global variable access
   - ✅ Internal function calls
   - ✅ Constructor execution
   - ✅ Weak symbols
   - ✅ IFUNC resolution
   - ✅ Thread-local storage
   - ✅ Multi-threaded TLS isolation

## Known Limitations

1. **SIMD/NEON**: The `arm64_simd_sum` function requires ARM NEON support. It falls back to scalar implementation if not available.

2. **IFUNC**: The IFUNC test uses a simple resolver. Real-world implementations would check CPU features (crypto extensions, SVE, etc.).

3. **TLS Models**: Currently tests only Local-Exec and Initial-Exec TLS models. General-Dynamic and Local-Dynamic are not yet covered.

4. **128-bit Operations**: R_AARCH64_LDST128_ABS_LO12_NC relocations are defined but not exercised (would require NEON/SVE code).

## Troubleshooting

### Test Library Won't Load

If you see errors like "cannot open shared object file":

```bash
# Verify the library exists and has correct architecture
file testdata/libarm64.so
# Should show: ELF 64-bit LSB shared object, ARM aarch64

# Check dependencies
ldd testdata/libarm64.so
```

### Wrong Architecture

If tests skip on x86_64:

```
--- SKIP: TestARM64LoadLibrary (0.00s)
    arm64_test.go:10: Test file only runs on arm64
```

This is expected. ARM64 tests only run on `GOARCH=arm64`.

### Relocation Failures

If you see "unknown relocation type" errors, it means:
1. The test library uses a relocation not yet implemented in `loader/reloc_arm64.go`
2. Check the error message for the relocation number
3. Add support in the relocation handler or adjust test library compilation

## Reference Documents

- [ARM ELF Specification](https://github.com/ARM-software/abi-aa/blob/main/aaelf64/aaelf64.rst)
- [ARM64 Relocations](https://github.com/ARM-software/abi-aa/blob/main/aaelf64/aaelf64.rst#relocation)
- [ARM TLS](https://github.com/ARM-software/abi-aa/blob/main/aaelf64/aaelf64.rst#thread-local-storage)
