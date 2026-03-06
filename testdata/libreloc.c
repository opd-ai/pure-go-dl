// Build: gcc -shared -fPIC -o libreloc.so libreloc.c
static int square(int x) { return x * x; }
int square_plus_one(int x) { return square(x) + 1; }
