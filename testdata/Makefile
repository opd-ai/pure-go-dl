.PHONY: all clean

CC ?= gcc
CFLAGS = -shared -fPIC -O2 -g

all: libtest.so libreloc.so libifunc.so libtls.so

libtest.so: libtest.c
	$(CC) $(CFLAGS) -o $@ $<

libreloc.so: libreloc.c
	$(CC) $(CFLAGS) -o $@ $<

libifunc.so: libifunc.c
	$(CC) $(CFLAGS) -o $@ $<

libtls.so: libtls.c
	$(CC) $(CFLAGS) -o $@ $<

clean:
	rm -f *.so
