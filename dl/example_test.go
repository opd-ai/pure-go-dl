package dl

import (
	"fmt"
	"log"
)

// Example demonstrates basic library loading and function calling.
// This example loads a shared library and calls a function from it.
func Example() {
	// Load a shared library
	lib, err := Open("../testdata/libtest.so")
	if err != nil {
		log.Fatal(err)
	}
	defer lib.Close()

	// Bind a function to a Go variable
	var add func(int, int) int
	if err := lib.Bind("add", &add); err != nil {
		log.Fatal(err)
	}

	// Call the native function
	result := add(2, 3)
	fmt.Printf("add(2, 3) = %d\n", result)
	// Output: add(2, 3) = 5
}

// Example_sym demonstrates symbol address lookup.
// This shows how to get the raw address of a symbol without binding it.
func Example_sym() {
	lib, err := Open("../testdata/libtest.so")
	if err != nil {
		log.Fatal(err)
	}
	defer lib.Close()

	// Look up a symbol address
	addr, err := lib.Sym("add")
	if err != nil {
		log.Fatal(err)
	}

	// The address is non-zero for existing symbols
	fmt.Printf("Symbol found: %t\n", addr != 0)
	// Output: Symbol found: true
}

// Example_flags demonstrates library loading with flags.
// RTLD_GLOBAL makes symbols available to subsequently loaded libraries.
func Example_flags() {
	// Load a library with RTLD_GLOBAL flag
	// This makes its symbols available to other libraries loaded later
	lib, err := Open("../testdata/libtest.so", RTLD_GLOBAL)
	if err != nil {
		log.Fatal(err)
	}
	defer lib.Close()

	// The library is now loaded
	fmt.Println("Library loaded with RTLD_GLOBAL")
	// Output: Library loaded with RTLD_GLOBAL
}

// Example_close demonstrates reference counting and cleanup.
// Libraries can be opened multiple times and are only unloaded when
// all references are closed.
func Example_close() {
	// First reference
	lib1, err := Open("../testdata/libtest.so")
	if err != nil {
		log.Fatal(err)
	}

	// Second reference to same library
	lib2, err := Open("../testdata/libtest.so")
	if err != nil {
		log.Fatal(err)
	}

	// Close first reference - library stays loaded
	lib1.Close()

	// Can still use second reference
	var add func(int, int) int
	if err := lib2.Bind("add", &add); err != nil {
		log.Fatal(err)
	}
	result := add(10, 20)
	fmt.Printf("add(10, 20) = %d\n", result)

	// Close second reference - library is now unloaded
	lib2.Close()
	// Output: add(10, 20) = 30
}
