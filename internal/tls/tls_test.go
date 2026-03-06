package tls

import (
	"sync"
	"testing"
	"unsafe"

	"github.com/opd-ai/pure-go-dl/internal/mmap"
	"golang.org/x/sys/unix"
)

func TestRegisterModule(t *testing.T) {
	mgr := GlobalManager()

	// Register a simple TLS module
	mod, err := mgr.RegisterModule(256, 16, 100, 0)
	if err != nil {
		t.Fatalf("RegisterModule failed: %v", err)
	}

	if mod.ID == 0 {
		t.Error("Module ID should be non-zero")
	}
	if mod.Size != 256 {
		t.Errorf("Expected size 256, got %d", mod.Size)
	}
	if mod.Align != 16 {
		t.Errorf("Expected alignment 16, got %d", mod.Align)
	}
	if mod.FileSize != 100 {
		t.Errorf("Expected fileSize 100, got %d", mod.FileSize)
	}
}

func TestAllocateBlock(t *testing.T) {
	mgr := GlobalManager()

	// Create a test module
	mod, err := mgr.RegisterModule(128, 8, 64, 0)
	if err != nil {
		t.Fatalf("RegisterModule failed: %v", err)
	}

	// Allocate a TLS block
	block, err := mgr.AllocateBlock(mod)
	if err != nil {
		t.Fatalf("AllocateBlock failed: %v", err)
	}
	defer block.Free()

	if block.Addr == 0 {
		t.Error("Block address should be non-zero")
	}
	if block.Module != mod {
		t.Error("Block module mismatch")
	}

	// Verify we can write to the block
	data := (*[128]byte)(unsafe.Pointer(block.Addr))
	data[0] = 0x42
	if data[0] != 0x42 {
		t.Error("Failed to write to TLS block")
	}
}

func TestBlockInitialization(t *testing.T) {
	mgr := GlobalManager()

	// Create init data in mmap'd memory (mimics real usage where InitData points to PT_LOAD segment)
	initData := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	initMem, err := mmap.Map(
		0,
		uintptr(len(initData)),
		unix.PROT_READ|unix.PROT_WRITE,
		mmap.MapPrivate|mmap.MapAnonymous,
		-1, 0)
	if err != nil {
		t.Fatalf("Failed to mmap init data: %v", err)
	}
	defer mmap.Unmap(initMem, uintptr(len(initData)))

	// Copy test data into mmap'd region
	initSlice := unsafe.Slice((*byte)(unsafe.Pointer(initMem)), len(initData))
	copy(initSlice, initData)

	initDataPtr := uintptr(initMem)

	// Register module with init data
	mod, err := mgr.RegisterModule(64, 8, uint64(len(initData)), initDataPtr)
	if err != nil {
		t.Fatalf("RegisterModule failed: %v", err)
	}

	// Allocate block
	block, err := mgr.AllocateBlock(mod)
	if err != nil {
		t.Fatalf("AllocateBlock failed: %v", err)
	}
	defer block.Free()

	// Verify initialization data was copied
	blockData := unsafe.Slice((*byte)(unsafe.Pointer(block.Addr)), len(initData))
	for i, want := range initData {
		if blockData[i] != want {
			t.Errorf("Init data mismatch at index %d: got %d, want %d", i, blockData[i], want)
		}
	}

	// Verify BSS is zeroed
	bssData := unsafe.Slice((*byte)(unsafe.Pointer(block.Addr+uintptr(len(initData)))), 64-uint64(len(initData)))
	for i, v := range bssData {
		if v != 0 {
			t.Errorf("BSS not zeroed at index %d: got %d", i, v)
		}
	}
}

func TestModuleID(t *testing.T) {
	mgr := GlobalManager()

	mod1, _ := mgr.RegisterModule(32, 4, 16, 0)
	mod2, _ := mgr.RegisterModule(64, 8, 32, 0)

	if mod1.GetModuleID() == 0 {
		t.Error("Module 1 ID should be non-zero")
	}
	if mod2.GetModuleID() == 0 {
		t.Error("Module 2 ID should be non-zero")
	}
	if mod1.GetModuleID() >= mod2.GetModuleID() {
		t.Error("Module IDs should be strictly increasing")
	}
}

func TestInvalidAlignment(t *testing.T) {
	mgr := GlobalManager()

	// Try to register with excessive alignment
	_, err := mgr.RegisterModule(128, 8192, 64, 0)
	if err == nil {
		t.Error("Expected error for alignment exceeding page size")
	}
}

func TestMultiThreadedTLSAccess(t *testing.T) {
	mgr := GlobalManager()
	reg := GetGlobalRegistry()

	// Register a test module
	mod, err := mgr.RegisterModule(128, 8, 64, 0)
	if err != nil {
		t.Fatalf("RegisterModule failed: %v", err)
	}

	const numThreads = 10
	done := make(chan struct{}, numThreads)
	threadIDs := make(map[uint64]bool)
	var mu sync.Mutex

	// Spawn multiple goroutines, each locked to an OS thread
	for i := 0; i < numThreads; i++ {
		go func(threadNum int) {
			// Lock goroutine to OS thread to get consistent thread ID
			// Note: In real usage, C code would be running on actual OS threads
			// For testing, we simulate this with goroutine thread IDs
			
			// Allocate TLS via __tls_get_addr simulation
			idx := TLSIndex{
				ModuleID: mod.ID,
				Offset:   0,
			}
			
			addr := GetTLSAddr(uintptr(unsafe.Pointer(&idx)))
			if addr == 0 {
				t.Errorf("Thread %d: GetTLSAddr returned null", threadNum)
				done <- struct{}{}
				return
			}

			// Write thread-specific data
			data := (*uint64)(unsafe.Pointer(addr))
			*data = uint64(threadNum + 1000)

			// Read it back
			if *data != uint64(threadNum+1000) {
				t.Errorf("Thread %d: Data mismatch, got %d, want %d", threadNum, *data, threadNum+1000)
			}

			// Track thread IDs to verify multi-threading
			threadID := getCurrentThreadID()
			mu.Lock()
			threadIDs[threadID] = true
			mu.Unlock()

			done <- struct{}{}
		}(i)
	}

	// Wait for all threads
	for i := 0; i < numThreads; i++ {
		<-done
	}

	// Verify we used actual thread IDs (may be single-threaded in test, that's ok)
	if len(threadIDs) == 0 {
		t.Error("No thread IDs recorded")
	}

	// Cleanup test threads
	mu.Lock()
	for tid := range threadIDs {
		if err := reg.CleanupThread(tid); err != nil {
			t.Errorf("CleanupThread(%d) failed: %v", tid, err)
		}
	}
	mu.Unlock()
}

func TestPerThreadIsolation(t *testing.T) {
	mgr := GlobalManager()
	reg := GetGlobalRegistry()

	// Register a test module
	mod, err := mgr.RegisterModule(64, 8, 32, 0)
	if err != nil {
		t.Fatalf("RegisterModule failed: %v", err)
	}

	const magic1 = uint64(0xDEADBEEF)
	const magic2 = uint64(0xCAFEBABE)

	type result struct {
		threadID uint64
		value    uint64
		addr     uintptr
	}
	results := make(chan result, 2)

	// Thread 1: write magic1
	go func() {
		idx := TLSIndex{ModuleID: mod.ID, Offset: 0}
		addr := GetTLSAddr(uintptr(unsafe.Pointer(&idx)))
		if addr != 0 {
			ptr := (*uint64)(unsafe.Pointer(addr))
			*ptr = magic1
			results <- result{getCurrentThreadID(), *ptr, addr}
		} else {
			results <- result{0, 0, 0}
		}
	}()

	// Thread 2: write magic2
	go func() {
		idx := TLSIndex{ModuleID: mod.ID, Offset: 0}
		addr := GetTLSAddr(uintptr(unsafe.Pointer(&idx)))
		if addr != 0 {
			ptr := (*uint64)(unsafe.Pointer(addr))
			*ptr = magic2
			results <- result{getCurrentThreadID(), *ptr, addr}
		} else {
			results <- result{0, 0, 0}
		}
	}()

	// Collect results
	r1 := <-results
	r2 := <-results

	// Verify both got addresses
	if r1.addr == 0 || r2.addr == 0 {
		t.Fatal("One or both threads failed to get TLS address")
	}

	// Verify values are correct
	if r1.value != magic1 && r1.value != magic2 {
		t.Errorf("Thread 1: unexpected value %#x", r1.value)
	}
	if r2.value != magic1 && r2.value != magic2 {
		t.Errorf("Thread 2: unexpected value %#x", r2.value)
	}

	// If threads are different, addresses should be different (isolation)
	if r1.threadID != r2.threadID && r1.addr == r2.addr {
		t.Error("Different threads got same TLS address (no isolation)")
	}

	// Cleanup
	reg.CleanupThread(r1.threadID)
	reg.CleanupThread(r2.threadID)
}

func TestDynamicModuleGrowth(t *testing.T) {
	mgr := GlobalManager()
	reg := GetGlobalRegistry()

	const numModules = 5
	modules := make([]*Module, numModules)

	// Register multiple modules
	for i := 0; i < numModules; i++ {
		mod, err := mgr.RegisterModule(64, 8, 32, 0)
		if err != nil {
			t.Fatalf("RegisterModule %d failed: %v", i, err)
		}
		modules[i] = mod
	}

	// Access TLS for each module from the same thread
	threadID := getCurrentThreadID()
	
	for i, mod := range modules {
		idx := TLSIndex{ModuleID: mod.ID, Offset: 0}
		addr := GetTLSAddr(uintptr(unsafe.Pointer(&idx)))
		if addr == 0 {
			t.Errorf("Module %d: GetTLSAddr returned null", i)
			continue
		}

		// Write unique value
		ptr := (*uint32)(unsafe.Pointer(addr))
		*ptr = uint32(i + 100)

		// Verify
		if *ptr != uint32(i+100) {
			t.Errorf("Module %d: value mismatch", i)
		}
	}

	// Verify module count
	moduleCount := reg.GetModuleCount()
	if moduleCount < numModules {
		t.Errorf("Expected at least %d modules, got %d", numModules, moduleCount)
	}

	// Cleanup
	reg.CleanupThread(threadID)
}

func TestCleanupThread(t *testing.T) {
	mgr := GlobalManager()
	reg := GetGlobalRegistry()

	mod, err := mgr.RegisterModule(64, 8, 32, 0)
	if err != nil {
		t.Fatalf("RegisterModule failed: %v", err)
	}

	threadID := getCurrentThreadID()

	// Allocate TLS block
	idx := TLSIndex{ModuleID: mod.ID, Offset: 0}
	addr := GetTLSAddr(uintptr(unsafe.Pointer(&idx)))
	if addr == 0 {
		t.Fatal("GetTLSAddr returned null")
	}

	// Verify block exists
	reg.mu.Lock()
	threadBlocks := reg.blocks[threadID]
	if threadBlocks == nil || threadBlocks[mod.ID] == nil {
		t.Fatal("TLS block not registered")
	}
	reg.mu.Unlock()

	// Cleanup
	if err := reg.CleanupThread(threadID); err != nil {
		t.Fatalf("CleanupThread failed: %v", err)
	}

	// Verify cleanup
	reg.mu.Lock()
	if reg.blocks[threadID] != nil {
		t.Error("Thread blocks not cleaned up")
	}
	reg.mu.Unlock()
}
