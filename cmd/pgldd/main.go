package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/opd-ai/pure-go-dl/dl"
)

func main() {
	flag.Parse()
	if flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: pgldd <library.so>")
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
