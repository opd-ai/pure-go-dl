.PHONY: all clean arm64

CC ?= gcc
CFLAGS = -shared -fPIC -O2 -g

all: libtest.so libreloc.so libifunc.so libtls.so libversion.so

# ARM64-specific libraries (only build on aarch64 or when explicitly requested)
arm64: libarm64.so libarm64_tls.so

libtest.so: libtest.c
	$(CC) $(CFLAGS) -o $@ $<

libreloc.so: libreloc.c
	$(CC) $(CFLAGS) -o $@ $<

libifunc.so: libifunc.c
	$(CC) $(CFLAGS) -o $@ $<

libtls.so: libtls.c
	$(CC) $(CFLAGS) -o $@ $<

libarm64.so: libarm64.c
	$(CC) $(CFLAGS) -o $@ $<

libarm64_tls.so: libarm64_tls.c
	$(CC) $(CFLAGS) -o $@ $< -lpthread

libversion.so: libversion.c libversion.map
	$(CC) $(CFLAGS) -Wl,--version-script=libversion.map -o $@ libversion.c

clean:
	rm -f *.so
