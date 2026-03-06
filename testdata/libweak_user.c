// Library that uses a symbol from another library
// Build: gcc -shared -fPIC -o libweak_user.so libweak_user.c

extern int get_value(void);

// Call the symbol - should resolve to strong version if available
int call_get_value(void) {
    return get_value();
}
