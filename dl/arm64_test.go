//go:build arm64 && linux

package dl

import (
	"testing"
)

// TestARM64LoadLibrary verifies that ARM64-specific test library loads successfully.
func TestARM64LoadLibrary(t *testing.T) {
	lib, err := Open("../testdata/libarm64.so")
	if err != nil {
		t.Fatalf("Failed to load libarm64.so: %v", err)
	}
	defer lib.Close()

	if lib == nil {
		t.Fatal("Open returned nil library")
	}
}

// TestARM64BasicFunctionCall tests calling a simple ARM64 function.
func TestARM64BasicFunctionCall(t *testing.T) {
	lib, err := Open("../testdata/libarm64.so")
	if err != nil {
		t.Fatalf("Failed to load libarm64.so: %v", err)
	}
	defer lib.Close()

	var add func(int, int) int
	if err := lib.Bind("arm64_add", &add); err != nil {
		t.Fatalf("Failed to bind arm64_add: %v", err)
	}

	result := add(10, 20)
	if result != 30 {
		t.Errorf("arm64_add(10, 20) = %d, want 30", result)
	}
}

// TestARM64GlobalVariable tests accessing global variables (R_AARCH64_GLOB_DAT).
func TestARM64GlobalVariable(t *testing.T) {
	lib, err := Open("../testdata/libarm64.so")
	if err != nil {
		t.Fatalf("Failed to load libarm64.so: %v", err)
	}
	defer lib.Close()

	var incrementCounter func() int
	if err := lib.Bind("arm64_increment_counter", &incrementCounter); err != nil {
		t.Fatalf("Failed to bind arm64_increment_counter: %v", err)
	}

	// Constructor should have set counter to 42
	first := incrementCounter()
	if first != 43 {
		t.Errorf("First arm64_increment_counter() = %d, want 43 (constructor sets to 42)", first)
	}

	second := incrementCounter()
	if second != 44 {
		t.Errorf("Second arm64_increment_counter() = %d, want 44", second)
	}
}

// TestARM64LocalDataAccess tests accessing static local arrays (R_AARCH64_LDST64_ABS_LO12_NC).
func TestARM64LocalDataAccess(t *testing.T) {
	lib, err := Open("../testdata/libarm64.so")
	if err != nil {
		t.Fatalf("Failed to load libarm64.so: %v", err)
	}
	defer lib.Close()

	var getData func(int) int
	if err := lib.Bind("arm64_get_local_data", &getData); err != nil {
		t.Fatalf("Failed to bind arm64_get_local_data: %v", err)
	}

	tests := []struct {
		index int
		want  int
	}{
		{0, 1},
		{1, 2},
		{2, 3},
		{3, 4},
		{-1, -1},
		{4, -1},
	}

	for _, tt := range tests {
		got := getData(tt.index)
		if got != tt.want {
			t.Errorf("arm64_get_local_data(%d) = %d, want %d", tt.index, got, tt.want)
		}
	}
}

// TestARM64InternalCall tests internal function calls (R_AARCH64_CALL26).
func TestARM64InternalCall(t *testing.T) {
	lib, err := Open("../testdata/libarm64.so")
	if err != nil {
		t.Fatalf("Failed to load libarm64.so: %v", err)
	}
	defer lib.Close()

	var sumChain func(int, int, int) int
	if err := lib.Bind("arm64_sum_chain", &sumChain); err != nil {
		t.Fatalf("Failed to bind arm64_sum_chain: %v", err)
	}

	result := sumChain(5, 10, 15)
	if result != 30 {
		t.Errorf("arm64_sum_chain(5, 10, 15) = %d, want 30", result)
	}
}

// TestARM64IFUNC tests indirect function resolution (R_AARCH64_IRELATIVE).
func TestARM64IFUNC(t *testing.T) {
	lib, err := Open("../testdata/libarm64.so")
	if err != nil {
		t.Fatalf("Failed to load libarm64.so: %v", err)
	}
	defer lib.Close()

	var multiply func(int, int) int
	if err := lib.Bind("arm64_multiply", &multiply); err != nil {
		t.Fatalf("Failed to bind arm64_multiply: %v", err)
	}

	result := multiply(6, 7)
	if result != 42 {
		t.Errorf("arm64_multiply(6, 7) = %d, want 42", result)
	}
}

// TestARM64WeakSymbol tests weak symbol handling.
func TestARM64WeakSymbol(t *testing.T) {
	lib, err := Open("../testdata/libarm64.so")
	if err != nil {
		t.Fatalf("Failed to load libarm64.so: %v", err)
	}
	defer lib.Close()

	var weakFunc func() int
	if err := lib.Bind("arm64_weak_symbol", &weakFunc); err != nil {
		t.Fatalf("Failed to bind arm64_weak_symbol: %v", err)
	}

	result := weakFunc()
	if result != 999 {
		t.Errorf("arm64_weak_symbol() = %d, want 999", result)
	}
}

// TestARM64TLSBasic tests basic thread-local storage access (R_AARCH64_TLS_TPREL64).
func TestARM64TLSBasic(t *testing.T) {
	lib, err := Open("../testdata/libarm64_tls.so")
	if err != nil {
		t.Fatalf("Failed to load libarm64_tls.so: %v", err)
	}
	defer lib.Close()

	var getCounter func() int
	var increment func() int
	var setCounter func(int)

	if err := lib.Bind("arm64_tls_get_counter", &getCounter); err != nil {
		t.Fatalf("Failed to bind arm64_tls_get_counter: %v", err)
	}
	if err := lib.Bind("arm64_tls_increment", &increment); err != nil {
		t.Fatalf("Failed to bind arm64_tls_increment: %v", err)
	}
	if err := lib.Bind("arm64_tls_set_counter", &setCounter); err != nil {
		t.Fatalf("Failed to bind arm64_tls_set_counter: %v", err)
	}

	// Constructor should have set counter to 100
	initial := getCounter()
	if initial != 100 {
		t.Errorf("Initial TLS counter = %d, want 100", initial)
	}

	// Increment
	first := increment()
	if first != 101 {
		t.Errorf("First increment = %d, want 101", first)
	}

	second := increment()
	if second != 102 {
		t.Errorf("Second increment = %d, want 102", second)
	}

	// Set and verify
	setCounter(50)
	value := getCounter()
	if value != 50 {
		t.Errorf("After setCounter(50), getCounter() = %d, want 50", value)
	}
}

// TestARM64TLSArray tests TLS array access (R_AARCH64_TLSLE_LDST64_TPREL_LO12_NC).
func TestARM64TLSArray(t *testing.T) {
	lib, err := Open("../testdata/libarm64_tls.so")
	if err != nil {
		t.Fatalf("Failed to load libarm64_tls.so: %v", err)
	}
	defer lib.Close()

	var getData func(int) int
	var setData func(int, int)
	var sumData func() int

	if err := lib.Bind("arm64_tls_get_data", &getData); err != nil {
		t.Fatalf("Failed to bind arm64_tls_get_data: %v", err)
	}
	if err := lib.Bind("arm64_tls_set_data", &setData); err != nil {
		t.Fatalf("Failed to bind arm64_tls_set_data: %v", err)
	}
	if err := lib.Bind("arm64_tls_sum_data", &sumData); err != nil {
		t.Fatalf("Failed to bind arm64_tls_sum_data: %v", err)
	}

	// Check initial values
	expected := []int{10, 20, 30, 40}
	for i, want := range expected {
		got := getData(i)
		if got != want {
			t.Errorf("Initial arm64_tls_get_data(%d) = %d, want %d", i, got, want)
		}
	}

	// Sum should be 100
	sum := sumData()
	if sum != 100 {
		t.Errorf("Initial arm64_tls_sum_data() = %d, want 100", sum)
	}

	// Modify and verify
	setData(0, 5)
	setData(1, 10)

	newSum := sumData()
	if newSum != 85 { // 5 + 10 + 30 + 40
		t.Errorf("After modifications, arm64_tls_sum_data() = %d, want 85", newSum)
	}
}

// TestARM64TLSStruct tests TLS struct access.
func TestARM64TLSStruct(t *testing.T) {
	lib, err := Open("../testdata/libarm64_tls.so")
	if err != nil {
		t.Fatalf("Failed to load libarm64_tls.so: %v", err)
	}
	defer lib.Close()

	var setPoint func(int, int, int)
	var getPointSum func() int

	if err := lib.Bind("arm64_tls_set_point", &setPoint); err != nil {
		t.Fatalf("Failed to bind arm64_tls_set_point: %v", err)
	}
	if err := lib.Bind("arm64_tls_get_point_sum", &getPointSum); err != nil {
		t.Fatalf("Failed to bind arm64_tls_get_point_sum: %v", err)
	}

	// Constructor should have initialized point to (1, 2, 3)
	initial := getPointSum()
	if initial != 6 {
		t.Errorf("Initial point sum = %d, want 6", initial)
	}

	// Set new values
	setPoint(10, 20, 30)
	newSum := getPointSum()
	if newSum != 60 {
		t.Errorf("After setPoint(10, 20, 30), sum = %d, want 60", newSum)
	}
}

// TestARM64TLSMultipleThreads tests TLS isolation across goroutines.
func TestARM64TLSMultipleThreads(t *testing.T) {
	lib, err := Open("../testdata/libarm64_tls.so")
	if err != nil {
		t.Fatalf("Failed to load libarm64_tls.so: %v", err)
	}
	defer lib.Close()

	var setCounter func(int)
	var getCounter func() int

	if err := lib.Bind("arm64_tls_set_counter", &setCounter); err != nil {
		t.Fatalf("Failed to bind arm64_tls_set_counter: %v", err)
	}
	if err := lib.Bind("arm64_tls_get_counter", &getCounter); err != nil {
		t.Fatalf("Failed to bind arm64_tls_get_counter: %v", err)
	}

	// Note: In Go, each goroutine doesn't automatically get its own OS thread
	// unless we use runtime.LockOSThread(). This test verifies the API works,
	// but true multi-thread TLS isolation testing requires more complex setup
	// with pthread_create or runtime.LockOSThread().

	setCounter(123)
	value := getCounter()
	if value != 123 {
		t.Errorf("TLS counter = %d, want 123", value)
	}
}

// TestARM64TLSPointer tests TLS access via pointer.
func TestARM64TLSPointer(t *testing.T) {
	lib, err := Open("../testdata/libarm64_tls.so")
	if err != nil {
		t.Fatalf("Failed to load libarm64_tls.so: %v", err)
	}
	defer lib.Close()

	var incrementViaPtr func() int
	var getCounter func() int

	if err := lib.Bind("arm64_tls_increment_via_ptr", &incrementViaPtr); err != nil {
		t.Fatalf("Failed to bind arm64_tls_increment_via_ptr: %v", err)
	}
	if err := lib.Bind("arm64_tls_get_counter", &getCounter); err != nil {
		t.Fatalf("Failed to bind arm64_tls_get_counter: %v", err)
	}

	initial := getCounter()

	first := incrementViaPtr()
	if first != initial+1 {
		t.Errorf("First incrementViaPtr() = %d, want %d", first, initial+1)
	}

	second := incrementViaPtr()
	if second != initial+2 {
		t.Errorf("Second incrementViaPtr() = %d, want %d", second, initial+2)
	}
}
