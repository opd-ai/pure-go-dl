# Test Libraries

This directory contains pre-compiled shared libraries used for integration testing.

## Files

- **libtest.so** — Simple test library with add/square functions and constructor/destructor
- **libreloc.so** — Tests internal function calls and relocations

## Why Pre-compiled?

The `.so` files are checked into git for:
- **Reproducibility**: Ensures all developers and CI systems test against identical binaries
- **No GCC dependency**: Tests can run without requiring a C compiler
- **Consistency**: Different GCC versions may produce different binaries with varying relocations

## Rebuilding

If you modify the C source files, rebuild with:

```bash
make -C testdata
```

**Build requirements:**
- GCC with `-shared -fPIC` support
- x86-64 Linux target

## Compiler Version

These libraries were built with:
```bash
gcc (Ubuntu 11.4.0-1ubuntu1~22.04) 11.4.0
```

Build flags: `-shared -fPIC -O2 -g`
