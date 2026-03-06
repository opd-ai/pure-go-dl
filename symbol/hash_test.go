package symbol_test

import (
	"testing"

	"github.com/opd-ai/pure-go-dl/symbol"
)

func TestSysvHash(t *testing.T) {
	tests := []struct {
		name string
	}{
		{""},
		{"_start"},
		{"printf"},
		{"malloc"},
		{"free"},
		{"__libc_start_main"},
	}
	// At minimum verify the hash doesn't panic and returns consistent results.
	for _, tt := range tests {
		h1 := symbol.SysvHash(tt.name)
		h2 := symbol.SysvHash(tt.name)
		if h1 != h2 {
			t.Errorf("SysvHash(%q) is not deterministic: %d vs %d", tt.name, h1, h2)
		}
	}
}

func TestGnuHash(t *testing.T) {
	tests := []struct {
		name string
		want uint32
	}{
		{"", 5381},
		{"printf", 0x156b2bb8},
	}
	for _, tt := range tests {
		got := symbol.GnuHash(tt.name)
		if got != tt.want {
			t.Errorf("GnuHash(%q) = 0x%x, want 0x%x", tt.name, got, tt.want)
		}
	}
}
