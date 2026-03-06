// Library that provides a strong symbol definition
// Build: gcc -shared -fPIC -o libstrong_provider.so libstrong_provider.c

#include <stdio.h>

// Strong symbol that should override weak symbols
int get_value(void) {
    return 200; // strong version returns 200
}
