// libarm64.c - ARM64-specific test library
//
// This library is designed to exercise ARM64-specific relocation types:
// - R_AARCH64_RELATIVE (position-independent code)
// - R_AARCH64_GLOB_DAT (global data access)
// - R_AARCH64_JUMP_SLOT (function calls)
// - R_AARCH64_ABS64 (64-bit absolute addresses)
// - R_AARCH64_CALL26 (direct function calls)
//
// To build on ARM64 Linux:
//   gcc -shared -fPIC -O2 -g -o libarm64.so libarm64.c

#include <stdint.h>

// Global variable (exercises R_AARCH64_GLOB_DAT)
int arm64_global_counter = 100;

// Global array (exercises R_AARCH64_RELATIVE for pointer arrays)
static int local_data[4] = {1, 2, 3, 4};

// Function pointer array (exercises R_AARCH64_ABS64)
static int (*func_ptrs[2])(int);

// Internal helper function
static int arm64_internal_add(int a, int b) {
    return a + b;
}

// Internal helper using global data
static int arm64_internal_sum_array(void) {
    int sum = 0;
    for (int i = 0; i < 4; i++) {
        sum += local_data[i];
    }
    return sum;
}

// Public function that calls internal function (exercises R_AARCH64_CALL26)
int arm64_add(int a, int b) {
    return arm64_internal_add(a, b);
}

// Public function accessing global variable (exercises R_AARCH64_ADR_PREL_PG_HI21 + ADD_ABS_LO12_NC)
int arm64_increment_counter(void) {
    return ++arm64_global_counter;
}

// Public function accessing local data array (exercises R_AARCH64_ADR_PREL_PG_HI21 + LDST64_ABS_LO12_NC)
int arm64_get_local_data(int index) {
    if (index < 0 || index >= 4) {
        return -1;
    }
    return local_data[index];
}

// Function using function pointer (exercises R_AARCH64_ADR_GOT_PAGE + LD64_GOT_LO12_NC)
int arm64_call_via_pointer(int value) {
    if (func_ptrs[0] != NULL) {
        return func_ptrs[0](value);
    }
    return value;
}

// Initialization function (exercises indirect calls)
void arm64_init_function_pointers(void) {
    func_ptrs[0] = arm64_internal_add;
    func_ptrs[1] = arm64_internal_sum_array;
}

// Constructor (exercises DT_INIT_ARRAY with ARM64 relocations)
__attribute__((constructor))
static void arm64_constructor(void) {
    arm64_global_counter = 42;
    arm64_init_function_pointers();
}

// Function returning sum (for testing R_AARCH64_CALL26 chains)
int arm64_sum_chain(int a, int b, int c) {
    int ab = arm64_add(a, b);
    int abc = arm64_add(ab, c);
    return abc;
}

// ARM64-specific: function using 128-bit load/store (exercises R_AARCH64_LDST128_ABS_LO12_NC)
// Note: This requires NEON/SIMD support
#ifdef __ARM_NEON
#include <arm_neon.h>

int64_t arm64_simd_sum(void) {
    int32x4_t vec = vld1q_s32(local_data);
    int64x2_t sum_vec = vpaddlq_s32(vec);
    int64_t result[2];
    vst1q_s64(result, sum_vec);
    return result[0] + result[1];
}
#else
int64_t arm64_simd_sum(void) {
    return arm64_internal_sum_array();
}
#endif

// Weak symbol (exercises weak symbol handling on ARM64)
__attribute__((weak))
int arm64_weak_symbol(void) {
    return 999;
}

// IFUNC resolver (exercises R_AARCH64_IRELATIVE)
// This allows runtime selection of optimized implementations
static int arm64_multiply_generic(int a, int b) {
    return a * b;
}

static int arm64_multiply_optimized(int a, int b) {
    // In a real implementation, this would use ARM64-specific instructions
    return a * b;
}

// IFUNC resolver: selects implementation at load time
__attribute__((ifunc("arm64_multiply_resolver")))
int arm64_multiply(int a, int b);

static void *arm64_multiply_resolver(void) {
    // In a real implementation, check CPU features
    // For this test, just return the generic version
    return arm64_multiply_generic;
}
