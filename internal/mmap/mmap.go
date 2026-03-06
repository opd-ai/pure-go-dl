package mmap

import (
	"fmt"

	"golang.org/x/sys/unix"
)

const (
	ProtNone  = unix.PROT_NONE
	ProtRead  = unix.PROT_READ
	ProtWrite = unix.PROT_WRITE
	ProtExec  = unix.PROT_EXEC

	MapPrivate   = unix.MAP_PRIVATE
	MapAnonymous = unix.MAP_ANONYMOUS
	// MapFixedFlag is the MAP_FIXED flag constant. The name avoids shadowing
	// the MapFixed helper function exported by this package.
	MapFixedFlag = unix.MAP_FIXED
	MapShared    = unix.MAP_SHARED
)

// Map creates a memory mapping. addr=0 lets the kernel choose.
func Map(addr, length uintptr, prot, flags int, fd int, offset int64) (uintptr, error) {
	ret, _, errno := unix.Syscall6(
		unix.SYS_MMAP,
		addr,
		length,
		uintptr(prot),
		uintptr(flags),
		uintptr(fd),
		uintptr(offset),
	)
	if errno != 0 {
		return 0, fmt.Errorf("mmap: %w", errno)
	}
	return ret, nil
}

// MapAnon creates an anonymous private memory mapping.
func MapAnon(length uintptr, prot int) (uintptr, error) {
	return Map(0, length, prot, unix.MAP_PRIVATE|unix.MAP_ANONYMOUS, -1, 0)
}

// MapFixed maps over an existing reservation at a fixed address.
func MapFixed(addr, length uintptr, prot, flags int, fd int, offset int64) (uintptr, error) {
	return Map(addr, length, prot, flags|unix.MAP_FIXED, fd, offset)
}

// Protect changes the protection of a memory region.
func Protect(addr, length uintptr, prot int) error {
	_, _, errno := unix.Syscall(unix.SYS_MPROTECT, addr, length, uintptr(prot))
	if errno != 0 {
		return fmt.Errorf("mprotect: %w", errno)
	}
	return nil
}

// Unmap removes a memory mapping.
func Unmap(addr, length uintptr) error {
	_, _, errno := unix.Syscall(unix.SYS_MUNMAP, addr, length, 0)
	if errno != 0 {
		return fmt.Errorf("munmap: %w", errno)
	}
	return nil
}
