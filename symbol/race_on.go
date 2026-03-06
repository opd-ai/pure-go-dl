//go:build race

package symbol

func init() {
	RaceEnabled = true
}
