package symbol_test

import (
	"testing"

	"github.com/opd-ai/pure-go-dl/symbol"
)

func TestSysvHash(t *testing.T) {
	tests := []struct {
		name string
		want uint32
	}{
		{"", 0x00000000},
		{"printf", 0x077905a6},
		{"malloc", 0x07383353},
		{"free", 0x0006d8b5},
		{"cos", 0x00006a63},
	}
	for _, tt := range tests {
		got := symbol.SysvHash(tt.name)
		if got != tt.want {
			t.Errorf("SysvHash(%q) = 0x%x, want 0x%x", tt.name, got, tt.want)
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
