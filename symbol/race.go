package symbol

// RaceEnabled is set to true when running with the -race flag.
// It's used by tests to skip synthetic memory tests that don't pass checkptr validation.
var RaceEnabled = false
