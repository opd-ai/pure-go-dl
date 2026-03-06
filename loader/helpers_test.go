package loader

import "testing"

// newMockResolver creates a new mockResolver for testing.
func newMockResolver() *mockResolver {
	return &mockResolver{symbols: make(map[string]uintptr)}
}

// loadTestLibrary loads the standard test library and returns it.
// It automatically registers cleanup on test completion.
func loadTestLibrary(t *testing.T) *Object {
	t.Helper()
	testLib := "../testdata/libtest.so"
	resolver := newMockResolver()
	obj, err := Load(testLib, resolver)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	t.Cleanup(func() { Unload(obj) })
	return obj
}

// loadTestLibraryWithResolver loads the test library with a custom resolver.
func loadTestLibraryWithResolver(t *testing.T, resolver SymbolResolver) *Object {
	t.Helper()
	testLib := "../testdata/libtest.so"
	obj, err := Load(testLib, resolver)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	t.Cleanup(func() { Unload(obj) })
	return obj
}
