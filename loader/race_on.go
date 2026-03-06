//go:build race

package loader

func init() {
	RaceEnabled = true
}
