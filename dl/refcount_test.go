package dl

import (
	"sync"
	"testing"
	"time"
)

// TestConcurrentRefCounting verifies that concurrent Open/Close operations
// on the same library maintain correct reference counts without races.
func TestConcurrentRefCounting(t *testing.T) {
	const goroutines = 20
	const iterations = 10

	var wg sync.WaitGroup
	errors := make(chan error, goroutines*iterations)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				lib, err := Open("../testdata/libtest.so")
				if err != nil {
					errors <- err
					return
				}
				// Small delay to increase chance of concurrent access
				time.Sleep(time.Microsecond)
				if err := lib.Close(); err != nil {
					errors <- err
					return
				}
			}
		}()
	}

	wg.Wait()
	close(errors)

	// Check for any errors
	for err := range errors {
		t.Errorf("Concurrent operation failed: %v", err)
	}

	// Verify final state - library should be unloaded
	mu.Lock()
	_, exists := loaded["../testdata/libtest.so"]
	mu.Unlock()

	if exists {
		t.Error("Library still loaded after all Close() calls")
	}
}

// TestRefCountingWithDependencies verifies reference counting works correctly
// when loading libraries with dependencies.
func TestRefCountingWithDependencies(t *testing.T) {
	// Load libreloc which depends on libtest
	lib1, err := Open("../testdata/libreloc.so")
	if err != nil {
		t.Fatalf("Failed to load libreloc: %v", err)
	}

	// Verify both libraries are loaded
	mu.Lock()
	libreloc, hasReloc := loaded["../testdata/libreloc.so"]
	// libtest might be loaded as a dependency
	mu.Unlock()

	if !hasReloc {
		t.Error("libreloc not in loaded map")
	}

	// Load the same library again
	lib2, err := Open("../testdata/libreloc.so")
	if err != nil {
		t.Fatalf("Second Open failed: %v", err)
	}

	if lib1.obj != lib2.obj {
		t.Error("Expected same object for duplicate Open")
	}

	expectedRefCount := 2
	if lib1.obj.RefCount != expectedRefCount {
		t.Errorf("RefCount = %d, want %d", lib1.obj.RefCount, expectedRefCount)
	}

	// Close first reference
	if err := lib1.Close(); err != nil {
		t.Errorf("First Close failed: %v", err)
	}

	mu.Lock()
	_, stillLoaded := loaded["../testdata/libreloc.so"]
	mu.Unlock()

	if !stillLoaded {
		t.Error("Library unloaded prematurely after first Close")
	}

	if libreloc.obj.RefCount != 1 {
		t.Errorf("After first Close, RefCount = %d, want 1", libreloc.obj.RefCount)
	}

	// Close second reference
	if err := lib2.Close(); err != nil {
		t.Errorf("Second Close failed: %v", err)
	}
}

// TestDoubleClose verifies that calling Close() too many times returns an error.
func TestDoubleClose(t *testing.T) {
	lib, err := Open("../testdata/libtest.so")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	// First close should succeed
	if err := lib.Close(); err != nil {
		t.Errorf("First Close failed: %v", err)
	}

	// Second close should fail
	err = lib.Close()
	if err == nil {
		t.Error("Expected error on double Close, got nil")
	}
	if err != nil && err.Error() != "dl: Close() called more than Open()" {
		t.Errorf("Expected specific error message, got: %v", err)
	}
}

// TestRefCountRaceDetector runs a race-prone scenario to catch issues with -race flag.
func TestRefCountRaceDetector(t *testing.T) {
	const goroutines = 10

	var wg sync.WaitGroup
	start := make(chan struct{})

	// All goroutines open the same library simultaneously
	libs := make([]*Library, goroutines)
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start // Wait for signal to start simultaneously
			lib, err := Open("../testdata/libtest.so")
			if err != nil {
				t.Errorf("Open failed: %v", err)
				return
			}
			libs[idx] = lib
		}(i)
	}

	close(start) // Signal all goroutines to start
	wg.Wait()

	// Verify reference count
	mu.Lock()
	lib, exists := loaded["../testdata/libtest.so"]
	mu.Unlock()

	if !exists {
		t.Fatal("Library not loaded")
	}

	if lib.obj.RefCount != goroutines {
		t.Errorf("RefCount = %d, want %d", lib.obj.RefCount, goroutines)
	}

	// Close all references
	for _, l := range libs {
		if l != nil {
			if err := l.Close(); err != nil {
				t.Errorf("Close failed: %v", err)
			}
		}
	}

	// Verify library is unloaded
	mu.Lock()
	_, stillExists := loaded["../testdata/libtest.so"]
	mu.Unlock()

	if stillExists {
		t.Error("Library still loaded after all Close() calls")
	}
}

// TestConcurrentLoadDifferentLibraries verifies concurrent loading of different libraries.
func TestConcurrentLoadDifferentLibraries(t *testing.T) {
	libraries := []string{
		"../testdata/libtest.so",
		"../testdata/libreloc.so",
	}

	var wg sync.WaitGroup
	errors := make(chan error, len(libraries)*10)

	for _, lib := range libraries {
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func(path string) {
				defer wg.Done()
				l, err := Open(path)
				if err != nil {
					errors <- err
					return
				}
				time.Sleep(time.Millisecond)
				if err := l.Close(); err != nil {
					errors <- err
				}
			}(lib)
		}
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("Concurrent load failed: %v", err)
	}
}

// TestRefCountingGlobalLibraries verifies reference counting for RTLD_GLOBAL libraries.
func TestRefCountingGlobalLibraries(t *testing.T) {
	lib1, err := Open("../testdata/libtest.so", RTLD_GLOBAL)
	if err != nil {
		t.Fatalf("Open with RTLD_GLOBAL failed: %v", err)
	}

	// Verify library is in globals list
	mu.Lock()
	globalCount := len(globals)
	foundInGlobals := false
	for _, g := range globals {
		if g == lib1 {
			foundInGlobals = true
			break
		}
	}
	mu.Unlock()

	if !foundInGlobals {
		t.Error("RTLD_GLOBAL library not in globals list")
	}

	// Open again
	lib2, err := Open("../testdata/libtest.so", RTLD_GLOBAL)
	if err != nil {
		t.Fatalf("Second Open failed: %v", err)
	}

	if lib1.obj != lib2.obj {
		t.Error("Expected same object for duplicate RTLD_GLOBAL Open")
	}

	// Close first reference
	if err := lib1.Close(); err != nil {
		t.Errorf("First Close failed: %v", err)
	}

	// Should still be in globals
	mu.Lock()
	stillInGlobals := false
	for _, g := range globals {
		if g.obj == lib1.obj {
			stillInGlobals = true
			break
		}
	}
	mu.Unlock()

	if !stillInGlobals {
		t.Error("Library removed from globals prematurely")
	}

	// Close second reference
	if err := lib2.Close(); err != nil {
		t.Errorf("Second Close failed: %v", err)
	}

	// Should now be removed from globals
	mu.Lock()
	finalGlobalCount := len(globals)
	mu.Unlock()

	if finalGlobalCount >= globalCount {
		t.Error("Library not removed from globals after final Close")
	}
}

// TestConcurrentOpenCloseStress performs a stress test with many goroutines
// rapidly opening and closing libraries.
func TestConcurrentOpenCloseStress(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	const goroutines = 50
	const duration = 2 * time.Second

	var wg sync.WaitGroup
	stop := make(chan struct{})
	errors := make(chan error, goroutines*100)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					lib, err := Open("../testdata/libtest.so")
					if err != nil {
						errors <- err
						return
					}
					if err := lib.Close(); err != nil {
						errors <- err
						return
					}
				}
			}
		}()
	}

	time.Sleep(duration)
	close(stop)
	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("Stress test error: %v", err)
	}

	// Verify clean final state - the specific library should be unloaded
	// Note: Other tests may have loaded other libraries, so we check specifically
	// for libtest.so rather than requiring all libraries to be unloaded
	mu.Lock()
	_, exists := loaded["../testdata/libtest.so"]
	mu.Unlock()

	if exists {
		t.Error("libtest.so still loaded after stress test")
	}
}

// TestRefCountWithSymbolLookup verifies reference counting while performing symbol lookups.
func TestRefCountWithSymbolLookup(t *testing.T) {
	lib1, err := Open("../testdata/libtest.so")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}

	// Lookup a symbol
	_, err = lib1.Sym("add")
	if err != nil {
		t.Errorf("Sym lookup failed: %v", err)
	}

	// Open again and lookup symbol from second handle
	lib2, err := Open("../testdata/libtest.so")
	if err != nil {
		t.Fatalf("Second Open failed: %v", err)
	}

	_, err = lib2.Sym("square_plus_one")
	if err != nil {
		t.Errorf("Sym lookup from second handle failed: %v", err)
	}

	// Both should reference the same object
	if lib1.obj != lib2.obj {
		t.Error("Expected same object")
	}

	if lib1.obj.RefCount != 2 {
		t.Errorf("RefCount = %d, want 2", lib1.obj.RefCount)
	}

	lib1.Close()
	lib2.Close()
}
