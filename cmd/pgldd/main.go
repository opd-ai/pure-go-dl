// Package main implements pgldd, a command-line tool that loads a shared library
// using pure-go-dl and prints its symbol table.
//
// pgldd demonstrates the library loading capabilities of pure-go-dl by loading
// an ELF shared object from a CGO_ENABLED=0 Go binary and displaying all exported
// symbols with their addresses and types.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/opd-ai/pure-go-dl/dl"
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `pgldd - Pure Go Dynamic Linker Diagnostic

Usage: pgldd <library.so>

Description:
  pgldd loads a shared library (.so file) using pure-go-dl and prints its
  symbol table. This demonstrates loading native shared libraries from a
  statically-linked Go binary (CGO_ENABLED=0).

  The tool displays all exported symbols with their addresses, sizes, binding
  types (LOCAL, GLOBAL, WEAK), and symbol types (FUNC, OBJECT, NOTYPE, etc.).

Example:
  pgldd /lib/x86_64-linux-gnu/libm.so.6
  pgldd testdata/libtest.so

Output Format:
  Each line shows: address, size, binding, type, and symbol name
  Example: 0x0000000000001130    23 GLOBAL FUNC cos

Options:
`)
		flag.PrintDefaults()
	}

	flag.Parse()
	if flag.NArg() == 0 {
		flag.Usage()
		os.Exit(1)
	}
	path := flag.Arg(0)
	lib, err := dl.Open(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer lib.Close()
	lib.PrintSymbols(os.Stdout)
}
