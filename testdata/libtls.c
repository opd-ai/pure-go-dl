// Test library with Thread-Local Storage (TLS) variables.
// Compile with: gcc -shared -fPIC -o libtls.so libtls.c

// Thread-local storage test variables
__thread int tls_counter = 42;
__thread char tls_buffer[256] = "Hello TLS";

// Function to get TLS counter value
int get_tls_counter(void) {
    return tls_counter;
}

// Function to set TLS counter value
void set_tls_counter(int value) {
    tls_counter = value;
}

// Function to get TLS buffer
const char* get_tls_buffer(void) {
    return tls_buffer;
}

// Function to set TLS buffer
void set_tls_buffer(const char* str) {
    int i = 0;
    while (str[i] != '\0' && i < 255) {
        tls_buffer[i] = str[i];
        i++;
    }
    tls_buffer[i] = '\0';
}

// Test function that modifies TLS variables
int increment_tls_counter(void) {
    tls_counter++;
    return tls_counter;
}
