// Simple test library
// Build: gcc -shared -fPIC -o libtest.so libtest.c

#include <stdio.h>

static int _counter = 0;

void __attribute__((constructor)) init_libtest(void) {
    _counter = 42;
}

void __attribute__((destructor)) fini_libtest(void) {
    _counter = 0;
}

int get_counter(void) {
    return _counter;
}

int add(int a, int b) {
    return a + b;
}

static int square(int x) { return x * x; }
int square_plus_one(int x) { return square(x) + 1; }

const char* hello(void) {
    return "Hello from libtest!";
}
