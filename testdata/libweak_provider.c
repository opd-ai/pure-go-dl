// Library that provides a weak symbol definition
// Build: gcc -shared -fPIC -o libweak_provider.so libweak_provider.c

#include <stdio.h>

// Weak symbol that can be overridden
__attribute__((weak)) int get_value(void) {
    return 100; // weak version returns 100
}
