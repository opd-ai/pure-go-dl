package mmap

import (
	"testing"
	"unsafe"

	"golang.org/x/sys/unix"
)

func TestMapAnon(t *testing.T) {
	// Test basic anonymous mapping
	pageSize := uintptr(unix.Getpagesize())
	addr, err := MapAnon(pageSize, ProtRead|ProtWrite)
	if err != nil {
		t.Fatalf("MapAnon failed: %v", err)
	}
	defer Unmap(addr, pageSize)

	if addr == 0 {
		t.Error("MapAnon returned zero address")
	}

	// Verify we can write to the mapped memory
	ptr := (*[1]byte)(unsafe.Pointer(addr))
	ptr[0] = 42
	if ptr[0] != 42 {
		t.Errorf("Write to mapped memory failed: got %d, want 42", ptr[0])
	}
}

func TestMapProtections(t *testing.T) {
	pageSize := uintptr(unix.Getpagesize())

	tests := []struct {
		name string
		prot int
	}{
		{"PROT_NONE", ProtNone},
		{"PROT_READ", ProtRead},
		{"PROT_WRITE", ProtWrite},
		{"PROT_READ|PROT_WRITE", ProtRead | ProtWrite},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, err := MapAnon(pageSize, tt.prot)
			if err != nil {
				t.Fatalf("MapAnon with %s failed: %v", tt.name, err)
			}
			defer Unmap(addr, pageSize)

			if addr == 0 {
				t.Error("MapAnon returned zero address")
			}
		})
	}
}

func TestProtect(t *testing.T) {
	pageSize := uintptr(unix.Getpagesize())

	// Create a read-write mapping
	addr, err := MapAnon(pageSize, ProtRead|ProtWrite)
	if err != nil {
		t.Fatalf("MapAnon failed: %v", err)
	}
	defer Unmap(addr, pageSize)

	// Write some data
	ptr := (*[4]byte)(unsafe.Pointer(addr))
	ptr[0] = 1
	ptr[1] = 2

	// Change protection to read-only
	err = Protect(addr, pageSize, ProtRead)
	if err != nil {
		t.Fatalf("Protect to PROT_READ failed: %v", err)
	}

	// Verify we can still read
	if ptr[0] != 1 || ptr[1] != 2 {
		t.Error("Cannot read after Protect to PROT_READ")
	}

	// Note: We can't easily test that writes now fail without causing a segfault
}

func TestUnmap(t *testing.T) {
	pageSize := uintptr(unix.Getpagesize())

	addr, err := MapAnon(pageSize, ProtRead|ProtWrite)
	if err != nil {
		t.Fatalf("MapAnon failed: %v", err)
	}

	// Unmap should succeed
	err = Unmap(addr, pageSize)
	if err != nil {
		t.Errorf("Unmap failed: %v", err)
	}

	// Double unmap behavior is platform-specific, so we don't test it
}

func TestMapFixed(t *testing.T) {
	pageSize := uintptr(unix.Getpagesize())

	// First create a reservation
	addr, err := MapAnon(pageSize, ProtNone)
	if err != nil {
		t.Fatalf("MapAnon reservation failed: %v", err)
	}
	defer Unmap(addr, pageSize)

	// Now map over it with MAP_FIXED
	fixedAddr, err := MapFixed(addr, pageSize, ProtRead|ProtWrite, MapPrivate|MapAnonymous, -1, 0)
	if err != nil {
		t.Fatalf("MapFixed failed: %v", err)
	}

	if fixedAddr != addr {
		t.Errorf("MapFixed returned %#x, want %#x", fixedAddr, addr)
	}

	// Verify we can use the fixed mapping
	ptr := (*[1]byte)(unsafe.Pointer(fixedAddr))
	ptr[0] = 99
	if ptr[0] != 99 {
		t.Error("Write to fixed mapping failed")
	}
}

func TestMap(t *testing.T) {
	pageSize := uintptr(unix.Getpagesize())

	// Test generic Map function with anonymous mapping
	addr, err := Map(0, pageSize, ProtRead|ProtWrite, MapPrivate|MapAnonymous, -1, 0)
	if err != nil {
		t.Fatalf("Map failed: %v", err)
	}
	defer Unmap(addr, pageSize)

	if addr == 0 {
		t.Error("Map returned zero address")
	}

	// Verify the mapping works
	ptr := (*[1]byte)(unsafe.Pointer(addr))
	ptr[0] = 123
	if ptr[0] != 123 {
		t.Error("Write to mapped memory failed")
	}
}

func TestMapInvalidSize(t *testing.T) {
	// Mapping zero bytes should fail
	_, err := MapAnon(0, ProtRead)
	if err == nil {
		t.Error("MapAnon with size 0 should fail")
	}
}

func TestProtectInvalidAddress(t *testing.T) {
	// Protecting an unmapped address should fail
	err := Protect(0x1000, 4096, ProtRead)
	if err == nil {
		t.Error("Protect on unmapped address should fail")
	}
}

func TestConstants(t *testing.T) {
	// Verify our constants match the unix package
	if ProtNone != unix.PROT_NONE {
		t.Errorf("ProtNone = %d, want %d", ProtNone, unix.PROT_NONE)
	}
	if ProtRead != unix.PROT_READ {
		t.Errorf("ProtRead = %d, want %d", ProtRead, unix.PROT_READ)
	}
	if ProtWrite != unix.PROT_WRITE {
		t.Errorf("ProtWrite = %d, want %d", ProtWrite, unix.PROT_WRITE)
	}
	if ProtExec != unix.PROT_EXEC {
		t.Errorf("ProtExec = %d, want %d", ProtExec, unix.PROT_EXEC)
	}
	if MapPrivate != unix.MAP_PRIVATE {
		t.Errorf("MapPrivate = %d, want %d", MapPrivate, unix.MAP_PRIVATE)
	}
	if MapAnonymous != unix.MAP_ANONYMOUS {
		t.Errorf("MapAnonymous = %d, want %d", MapAnonymous, unix.MAP_ANONYMOUS)
	}
	if MapFixedFlag != unix.MAP_FIXED {
		t.Errorf("MapFixedFlag = %d, want %d", MapFixedFlag, unix.MAP_FIXED)
	}
}
