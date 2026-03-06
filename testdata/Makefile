.PHONY: all clean

CC ?= gcc
CFLAGS = -shared -fPIC -O2 -g

all: libtest.so libreloc.so

libtest.so: libtest.c
	$(CC) $(CFLAGS) -o $@ $<

libreloc.so: libreloc.c
	$(CC) $(CFLAGS) -o $@ $<

clean:
	rm -f *.so
