package tls

import (
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

// TLSIndex is the structure passed to __tls_get_addr.
// It contains the module ID and offset within the TLS block.
type TLSIndex struct {
	ModuleID uint64
	Offset   uint64
}

// ThreadLocalRegistry manages per-thread TLS blocks.
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

// getCurrentThreadID returns a pseudo thread ID.
// In a real implementation, this would use gettid() or similar.
// For simplicity, we use goroutine ID approximation.
func getCurrentThreadID() uint64 {
	// Simplified: use a single "main thread" ID for now
	// In a full implementation, this would need proper thread tracking
	return 1
}

// GetTLSAddr implements the __tls_get_addr function.
// It takes a pointer to TLSIndex and returns the address of the TLS variable.
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
