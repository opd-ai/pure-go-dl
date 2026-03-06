// libifunc.c - Test library demonstrating IFUNC (indirect function) support.
//
// IFUNC allows runtime selection of function implementations based on CPU
// features or other runtime conditions. The resolver function is called once
// during relocation to determine which implementation to use.

#include <stdint.h>

// Two different implementations of the same function
static int add_impl_v1(int a, int b) {
    return a + b;
}

static int add_impl_v2(int a, int b) {
    return a + b + 100;  // Intentionally different for testing
}

// Resolver function: chooses which implementation to use.
// In real code, this would check CPU features (e.g., AVX support).
// For testing, we always return v1.
static void* add_resolver(void) {
    return (void*)add_impl_v1;
}

// The IFUNC symbol: calls add_resolver() to get the real function.
int add_ifunc(int a, int b) __attribute__((ifunc("add_resolver")));

// A simple non-IFUNC function for comparison
int multiply(int a, int b) {
    return a * b;
}
