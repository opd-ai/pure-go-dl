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

// findLibrary resolves a library name to an absolute file path.
//
// Search order:
//  1. LD_LIBRARY_PATH (colon-separated)
//  2. Default system paths
func findLibrary(name string) (string, error) {
	// If name is already an absolute or relative path, use it directly.
	if filepath.IsAbs(name) || strings.ContainsRune(name, '/') {
		if _, err := os.Stat(name); err == nil {
			return name, nil
		}
		return "", fmt.Errorf("dl: library not found: %q", name)
	}

	var searchPaths []string

	// 1. LD_LIBRARY_PATH
	if ldPath := os.Getenv("LD_LIBRARY_PATH"); ldPath != "" {
		for _, p := range strings.Split(ldPath, ":") {
			if p != "" {
				searchPaths = append(searchPaths, p)
			}
		}
	}

	// 2. Default system paths
	searchPaths = append(searchPaths, defaultSearchPaths...)

	for _, dir := range searchPaths {
		candidate := filepath.Join(dir, name)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("dl: library %q not found in search paths", name)
}
