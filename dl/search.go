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
//  3. /etc/ld.so.cache lookup
//  4. DT_RPATH of the requesting library (if provided)
//  5. Default system paths
//
// runpath and rpath are colon-separated path lists from DT_RUNPATH/DT_RPATH
// of the requesting library. They may be empty.
func findLibrary(name, runpath, rpath string) (string, error) {
	if path, err := checkDirectPath(name); path != "" || err != nil {
		return path, err
	}

	searchPaths := buildSearchPaths(runpath, rpath)
	if path := searchInPaths(name, searchPaths); path != "" {
		return path, nil
	}

	return "", fmt.Errorf("dl: library %q not found in search paths", name)
}

// checkDirectPath returns the path if name is absolute or contains a slash.
// Returns ("", nil) if not a direct path; (path, nil) if found; ("", error) if not found.
func checkDirectPath(name string) (string, error) {
	if !filepath.IsAbs(name) && !strings.ContainsRune(name, '/') {
		return "", nil // Not a direct path, continue searching
	}
	if _, err := os.Stat(name); err == nil {
		return name, nil
	}
	return "", fmt.Errorf("dl: library not found: %q", name)
}

// buildSearchPaths constructs the ordered list of directories to search.
func buildSearchPaths(runpath, rpath string) []struct {
	dirs     []string
	useCache bool
} {
	return []struct {
		dirs     []string
		useCache bool
	}{
		{splitPaths(runpath), false},                      // 1. DT_RUNPATH
		{splitPaths(os.Getenv("LD_LIBRARY_PATH")), false}, // 2. LD_LIBRARY_PATH
		{nil, true},                 // 3. /etc/ld.so.cache
		{splitPaths(rpath), false},  // 4. DT_RPATH
		{defaultSearchPaths, false}, // 5. Default paths
	}
}

// searchInPaths searches for name in the ordered list of search locations.
func searchInPaths(name string, searchPaths []struct {
	dirs     []string
	useCache bool
},
) string {
	for _, sp := range searchPaths {
		if path := searchInSearchPath(name, sp); path != "" {
			return path
		}
	}
	return ""
}

// searchInSearchPath searches for a library in a single search path.
func searchInSearchPath(name string, sp struct {
	dirs     []string
	useCache bool
},
) string {
	if sp.useCache {
		return searchInCacheIfExists(name)
	}
	return searchInDirs(name, sp.dirs)
}

// searchInCacheIfExists looks up a library in the cache and verifies it exists.
func searchInCacheIfExists(name string) string {
	cachedPath := lookupInCache(name)
	if cachedPath == "" {
		return ""
	}
	if _, err := os.Stat(cachedPath); err == nil {
		return cachedPath
	}
	return ""
}

// searchInDirs searches for name in a list of directories.
func searchInDirs(name string, dirs []string) string {
	for _, dir := range dirs {
		candidate := filepath.Join(dir, name)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}
