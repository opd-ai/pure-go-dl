package tls

import (
	"sync"
	"syscall"
	"unsafe"

	"github.com/ebitengine/purego"
)

// TLSIndex is the structure passed to __tls_get_addr.
// It contains the module ID and offset within the TLS block.
type TLSIndex struct {
	ModuleID uint64
	Offset   uint64
}

// ThreadLocalRegistry manages per-thread TLS blocks (Dynamic Thread Vector - DTV).
// Each OS thread has its own set of TLS blocks, one per module.
type ThreadLocalRegistry struct {
	mu     sync.Mutex
	blocks map[uint64]map[uint64]*Block // [threadID][moduleID] -> Block
}

var (
	globalRegistry     *ThreadLocalRegistry
	globalRegistryOnce sync.Once
	tlsGetAddrFunc     uintptr
)

// GetGlobalRegistry returns the singleton thread-local registry.
func GetGlobalRegistry() *ThreadLocalRegistry {
	globalRegistryOnce.Do(func() {
		globalRegistry = &ThreadLocalRegistry{
			blocks: make(map[uint64]map[uint64]*Block),
		}
	})
	return globalRegistry
}

// CleanupThread frees all TLS blocks for the given thread ID.
// This should be called when a thread exits to prevent memory leaks.
func (r *ThreadLocalRegistry) CleanupThread(threadID uint64) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	threadBlocks, ok := r.blocks[threadID]
	if !ok {
		return nil
	}

	var firstErr error
	for _, block := range threadBlocks {
		if err := block.Free(); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	delete(r.blocks, threadID)
	return firstErr
}

// GetModuleCount returns the number of TLS modules registered across all threads.
// This is useful for DTV management and debugging.
func (r *ThreadLocalRegistry) GetModuleCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	maxModuleID := uint64(0)
	for _, threadBlocks := range r.blocks {
		for moduleID := range threadBlocks {
			if moduleID > maxModuleID {
				maxModuleID = moduleID
			}
		}
	}
	return int(maxModuleID)
}

// getCurrentThreadID returns the current OS thread ID.
// This uses the Linux gettid() syscall to get the actual thread ID (TID).
// Each OS thread gets a unique ID, enabling proper per-thread TLS storage.
func getCurrentThreadID() uint64 {
	return uint64(syscall.Gettid())
}

// GetTLSAddr implements the __tls_get_addr function.
// It takes a pointer to TLSIndex and returns the address of the TLS variable.
//
//go:nocheckptr
func GetTLSAddr(indexPtr uintptr) uintptr {
	idx := (*TLSIndex)(unsafe.Pointer(indexPtr))

	reg := GetGlobalRegistry()
	threadID := getCurrentThreadID()

	reg.mu.Lock()
	defer reg.mu.Unlock()

	// Get or create the thread's block map
	threadBlocks, ok := reg.blocks[threadID]
	if !ok {
		threadBlocks = make(map[uint64]*Block)
		reg.blocks[threadID] = threadBlocks
	}

	// Get or create the TLS block for this module
	block, ok := threadBlocks[idx.ModuleID]
	if !ok {
		// Allocate a new block for this module
		mgr := GlobalManager()

		// Find the module
		mgr.mu.Lock()
		var mod *Module
		if idx.ModuleID > 0 && int(idx.ModuleID) <= len(mgr.modules) {
			mod = mgr.modules[idx.ModuleID-1]
		}
		mgr.mu.Unlock()

		if mod == nil {
			// Module not found
			return 0
		}

		var err error
		block, err = mgr.AllocateBlock(mod)
		if err != nil {
			return 0
		}

		threadBlocks[idx.ModuleID] = block
	}

	// Return the address of the variable at the given offset
	return block.Addr + uintptr(idx.Offset)
}

// RegisterTLSGetAddr returns a C-callable function pointer for __tls_get_addr.
// This creates a callback using purego that allows C code to call into our
// Go TLS implementation. The function follows the System V AMD64 ABI.
func RegisterTLSGetAddr() uintptr {
	if tlsGetAddrFunc != 0 {
		return tlsGetAddrFunc
	}

	// Create a C-callable callback for GetTLSAddr.
	// purego.NewCallback creates a function pointer that can be called from C code.
	// The signature must match: uintptr __tls_get_addr(uintptr ti)
	tlsGetAddrFunc = purego.NewCallback(GetTLSAddr)

	return tlsGetAddrFunc
}
