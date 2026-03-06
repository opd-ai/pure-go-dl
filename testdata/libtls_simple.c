// Simple TLS test using Initial-Exec model (simpler than General Dynamic)
// Compile with: gcc -shared -fPIC -ftls-model=initial-exec -o libtls_simple.so libtls_simple.c

// Note: initial-exec may not work in .so files on all systems
// This is a test to see what relocations we get

__attribute__((tls_model("initial-exec")))  
__thread int tls_counter = 42;

int get_tls_counter_simple(void) {
    return tls_counter;
}

void set_tls_counter_simple(int value) {
    tls_counter = value;
}
