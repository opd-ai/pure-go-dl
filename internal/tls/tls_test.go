package tls

import (
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
