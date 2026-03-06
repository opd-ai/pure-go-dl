// Package tls provides Thread-Local Storage (TLS) management for dynamically loaded
// shared libraries. It implements TLS block allocation, the Dynamic Thread Vector (DTV),
// and supports the General Dynamic (GD) TLS access model.
package tls

import (
	"fmt"
	"sync"
	"unsafe"

	"github.com/opd-ai/pure-go-dl/internal/mmap"
	"golang.org/x/sys/unix"
)

// Module represents a TLS template for a loaded shared library.
// Each library with a PT_TLS segment gets a TLS module with a unique ID.
type Module struct {
	ID       uint64  // Module ID (index into DTV)
	Size     uint64  // Size of TLS data + bss
	Align    uint64  // Required alignment
	FileSize uint64  // Size of initialized data
	InitData uintptr // Pointer to initialization data in mmap'd memory
}

// Block represents an allocated TLS block for a specific module and thread.
type Block struct {
	Addr   uintptr // Base address of the allocated TLS data
	Module *Module // Associated TLS module
}

// Manager manages TLS modules and allocates TLS blocks per thread.
type Manager struct {
	mu      sync.Mutex
	modules []*Module // Indexed by module ID (ID-1)
	nextID  uint64    // Next module ID to assign
}

var (
	globalManager     *Manager
	globalManagerOnce sync.Once
)

// GlobalManager returns the singleton TLS manager instance.
func GlobalManager() *Manager {
	globalManagerOnce.Do(func() {
		globalManager = &Manager{
			modules: make([]*Module, 0, 16),
			nextID:  1, // Module IDs start at 1
		}
	})
	return globalManager
}

// RegisterModule allocates a new TLS module ID and stores the TLS template.
// Returns the module ID, which is used in TLS relocations.
func (m *Manager) RegisterModule(size, align, fileSize uint64, initData uintptr) (*Module, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if align == 0 {
		align = 1
	}
	if align > 4096 {
		return nil, fmt.Errorf("tls: alignment %d exceeds page size", align)
	}

	mod := &Module{
		ID:       m.nextID,
		Size:     size,
		Align:    align,
		FileSize: fileSize,
		InitData: initData,
	}

	m.modules = append(m.modules, mod)
	m.nextID++

	return mod, nil
}

// AllocateBlock allocates a TLS block for the given module on the current thread.
// The block is initialized with data from the module's template.
func (m *Manager) AllocateBlock(mod *Module) (*Block, error) {
	if mod == nil {
		return nil, fmt.Errorf("tls: nil module")
	}

	// Allocate aligned memory for the TLS block.
	// Round up size to alignment boundary.
	alignedSize := (mod.Size + mod.Align - 1) &^ (mod.Align - 1)

	// Use mmap for allocation to get aligned memory.
	addr, err := mmap.Map(0, uintptr(alignedSize),
		mmap.ProtRead|mmap.ProtWrite,
		mmap.MapPrivate|mmap.MapAnonymous,
		-1, 0)
	if err != nil {
		return nil, fmt.Errorf("tls: mmap block failed: %w", err)
	}

	block := &Block{
		Addr:   uintptr(addr),
		Module: mod,
	}

	// Copy initialization data if present.
	if mod.FileSize > 0 && mod.InitData != 0 {
		initSlice := unsafe.Slice((*byte)(unsafe.Pointer(mod.InitData)), mod.FileSize)
		destSlice := unsafe.Slice((*byte)(unsafe.Pointer(block.Addr)), mod.FileSize)
		copy(destSlice, initSlice)
	}

	// Zero the remaining space (BSS portion).
	if mod.Size > mod.FileSize {
		bssStart := block.Addr + uintptr(mod.FileSize)
		bssSize := mod.Size - mod.FileSize
		bssSlice := unsafe.Slice((*byte)(unsafe.Pointer(bssStart)), bssSize)
		for i := range bssSlice {
			bssSlice[i] = 0
		}
	}

	return block, nil
}

// FreeBlock deallocates a TLS block.
func (b *Block) Free() error {
	if b.Addr == 0 {
		return nil
	}

	alignedSize := (b.Module.Size + b.Module.Align - 1) &^ (b.Module.Align - 1)
	err := mmap.Unmap(b.Addr, uintptr(alignedSize))
	b.Addr = 0
	return err
}

// GetModuleTLSOffset returns the offset of a TLS variable within its module.
// This is used for DTPOFF relocations.
func (mod *Module) GetModuleTLSOffset(offset int64) int64 {
	return offset
}

// GetThreadPointerOffset returns the offset from the thread pointer (TP)
// for a TLS variable. This is used for TPOFF relocations.
// For the General Dynamic model, this requires the block to be allocated.
func (b *Block) GetThreadPointerOffset() int64 {
	// In x86-64, thread pointer points to the end of the TLS block.
	// Negative offsets are used for TLS variables.
	// This is a simplified implementation - in reality, the thread pointer
	// is managed by the runtime and involves fs_base or gs_base.
	return -int64(b.Module.Size)
}

// GetModuleID returns the module ID for DTPMOD relocations.
func (mod *Module) GetModuleID() uint64 {
	return mod.ID
}

// pageAlign rounds v up to the next page boundary.
func pageAlign(v uint64) uint64 {
	pageSize := uint64(unix.Getpagesize())
	return (v + pageSize - 1) &^ (pageSize - 1)
}
