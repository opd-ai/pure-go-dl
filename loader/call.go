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

// CallIfuncResolver invokes an IFUNC resolver function at addr and returns
// the resolved function address. IFUNC resolvers are C-ABI functions that
// return uintptr (the address of the actual implementation to use).
// The resolver typically checks CPU features and returns an optimized variant.
//
// This function is exported for use by the dl package when resolving
// STT_GNU_IFUNC symbols during symbol lookup.
func CallIfuncResolver(addr uintptr) uintptr {
	var resolver func() uintptr
	purego.RegisterFunc(&resolver, addr)
	return resolver()
}
