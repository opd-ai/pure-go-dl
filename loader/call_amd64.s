// call_amd64.s - trampoline to call a C function with no arguments.

#include "textflag.h"

// func callFunc(addr uintptr)
// Calls the function at addr with no arguments, preserving Go ABI.
TEXT ·callFunc(SB),NOSPLIT,$0-8
    MOVQ addr+0(FP), AX
    CALL AX
    RET
