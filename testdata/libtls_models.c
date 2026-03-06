// Test library with different TLS access models to exercise code sequence relocations.
// Compile with different flags to test various TLS relocation types.

// General Dynamic model (default for shared libraries)
// This should generate TLSGD relocations if compiled appropriately
__thread int tls_gd_var = 100;

// Initial Exec model - generates GOTTPOFF relocations
__attribute__((tls_model("initial-exec")))  
__thread int tls_ie_var = 200;

// Local Dynamic model - for testing TLSLD
// Multiple TLS variables in same module should use LD model
__thread int tls_ld_var1 = 300;
__thread int tls_ld_var2 = 400;

// Accessor functions for GD model variable
int get_tls_gd(void) {
    return tls_gd_var;
}

void set_tls_gd(int val) {
    tls_gd_var = val;
}

// Accessor functions for IE model variable  
int get_tls_ie(void) {
    return tls_ie_var;
}

void set_tls_ie(int val) {
    tls_ie_var = val;
}

// Accessor functions for LD model variables
int get_tls_ld1(void) {
    return tls_ld_var1;
}

void set_tls_ld1(int val) {
    tls_ld_var1 = val;
}

int get_tls_ld2(void) {
    return tls_ld_var2;
}

void set_tls_ld2(int val) {
    tls_ld_var2 = val;
}

// Test function that uses multiple TLS variables
int sum_all_tls(void) {
    return tls_gd_var + tls_ie_var + tls_ld_var1 + tls_ld_var2;
}
