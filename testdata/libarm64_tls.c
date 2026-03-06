// libarm64_tls.c - ARM64 Thread-Local Storage test library
//
// This library exercises ARM64-specific TLS relocation types:
// - R_AARCH64_TLS_TPREL64 (thread-local offset)
// - R_AARCH64_TLSLE_ADD_TPREL_HI12 (TLS local exec, high 12 bits)
// - R_AARCH64_TLSLE_ADD_TPREL_LO12_NC (TLS local exec, low 12 bits)
// - R_AARCH64_TLSIE_LD64_GOTTPREL_LO12_NC (TLS initial exec)
//
// To build on ARM64 Linux:
//   gcc -shared -fPIC -O2 -g -o libarm64_tls.so libarm64_tls.c -lpthread

#include <pthread.h>
#include <stdint.h>

// Thread-local variables (exercises R_AARCH64_TLS_TPREL64)
__thread int arm64_tls_counter = 0;
__thread int arm64_tls_data[4] = {10, 20, 30, 40};

// Get TLS counter value
int arm64_tls_get_counter(void) {
    return arm64_tls_counter;
}

// Increment TLS counter
int arm64_tls_increment(void) {
    return ++arm64_tls_counter;
}

// Set TLS counter
void arm64_tls_set_counter(int value) {
    arm64_tls_counter = value;
}

// Get TLS array element (exercises R_AARCH64_TLSLE_LDST64_TPREL_LO12_NC)
int arm64_tls_get_data(int index) {
    if (index < 0 || index >= 4) {
        return -1;
    }
    return arm64_tls_data[index];
}

// Set TLS array element
void arm64_tls_set_data(int index, int value) {
    if (index >= 0 && index < 4) {
        arm64_tls_data[index] = value;
    }
}

// Sum TLS array (exercises multiple TLS accesses)
int arm64_tls_sum_data(void) {
    int sum = 0;
    for (int i = 0; i < 4; i++) {
        sum += arm64_tls_data[i];
    }
    return sum;
}

// Pointer to TLS variable (exercises R_AARCH64_TLSIE_ADR_GOTTPREL_PAGE21 + LD64_GOTTPREL_LO12_NC)
int *arm64_tls_get_counter_ptr(void) {
    return &arm64_tls_counter;
}

// Function that modifies TLS via pointer
int arm64_tls_increment_via_ptr(void) {
    int *ptr = arm64_tls_get_counter_ptr();
    return ++(*ptr);
}

// Struct with TLS members (exercises complex TLS layouts)
typedef struct {
    int x;
    int y;
    int z;
} arm64_tls_point_t;

__thread arm64_tls_point_t arm64_tls_point = {0, 0, 0};

void arm64_tls_set_point(int x, int y, int z) {
    arm64_tls_point.x = x;
    arm64_tls_point.y = y;
    arm64_tls_point.z = z;
}

int arm64_tls_get_point_sum(void) {
    return arm64_tls_point.x + arm64_tls_point.y + arm64_tls_point.z;
}

// Constructor that initializes TLS (exercises TLS in constructors)
__attribute__((constructor))
static void arm64_tls_constructor(void) {
    arm64_tls_counter = 100;
    arm64_tls_point.x = 1;
    arm64_tls_point.y = 2;
    arm64_tls_point.z = 3;
}
