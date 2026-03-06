// Test library with multiple symbol versions using GNU symbol versioning

#include <stdio.h>

// Version 1.0 implementation
__asm__(".symver add_v1, add@TESTLIB_1.0");
int add_v1(int a, int b) {
    return a + b;
}

// Version 2.0 implementation (default)
__asm__(".symver add_v2, add@@TESTLIB_2.0");
int add_v2(int a, int b) {
    return a + b + 100;  // Different behavior
}

// Unversioned symbol for comparison
int multiply(int a, int b) {
    return a * b;
}

// Symbol only in version 2.0
__asm__(".symver subtract_v2, subtract@@TESTLIB_2.0");
int subtract_v2(int a, int b) {
    return a - b;
}
