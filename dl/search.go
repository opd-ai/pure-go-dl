package dl

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// defaultSearchPaths are the standard library directories for x86-64 Linux.
var defaultSearchPaths = []string{
	"/lib/x86_64-linux-gnu",
	"/usr/lib/x86_64-linux-gnu",
	"/lib64",
	"/usr/lib64",
	"/lib",
	"/usr/lib",
}

// splitPaths splits a colon-separated path string and returns non-empty paths.
func splitPaths(paths string) []string {
	if paths == "" {
		return nil
	}
	var result []string
	for _, p := range strings.Split(paths, ":") {
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// findLibrary resolves a library name to an absolute file path.
//
// Search order (per ELF spec and ROADMAP Phase 5.1):
//  1. DT_RUNPATH of the requesting library (if provided)
//  2. LD_LIBRARY_PATH environment variable
//  3. DT_RPATH of the requesting library (if provided)
//  4. Default system paths
//
// runpath and rpath are colon-separated path lists from DT_RUNPATH/DT_RPATH
// of the requesting library. They may be empty.
func findLibrary(name, runpath, rpath string) (string, error) {
	// If name is already an absolute or relative path, use it directly.
	if filepath.IsAbs(name) || strings.ContainsRune(name, '/') {
		if _, err := os.Stat(name); err == nil {
			return name, nil
		}
		return "", fmt.Errorf("dl: library not found: %q", name)
	}

	var searchPaths []string

	// 1. DT_RUNPATH (modern, takes precedence over LD_LIBRARY_PATH)
	searchPaths = append(searchPaths, splitPaths(runpath)...)

	// 2. LD_LIBRARY_PATH
	searchPaths = append(searchPaths, splitPaths(os.Getenv("LD_LIBRARY_PATH"))...)

	// 3. DT_RPATH (legacy, lower priority than LD_LIBRARY_PATH)
	searchPaths = append(searchPaths, splitPaths(rpath)...)

	// 4. Default system paths
	searchPaths = append(searchPaths, defaultSearchPaths...)

	for _, dir := range searchPaths {
		candidate := filepath.Join(dir, name)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("dl: library %q not found in search paths", name)
}
