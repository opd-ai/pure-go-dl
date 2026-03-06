package loader

import "github.com/ebitengine/purego"

// callFunc invokes the C-ABI function at addr with no arguments.
// purego handles system-stack switching so the C function runs with
// a proper C stack, not the goroutine stack, satisfying the System V
// AMD64 ABI requirements for stack alignment and red-zone avoidance.
func callFunc(addr uintptr) {
	var fn func()
	purego.RegisterFunc(&fn, addr)
	fn()
}
