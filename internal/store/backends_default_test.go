//go:build !embedded

package store

// Default builds have no embedded driver compiled in, so the parity suite runs
// against the shellout backend only.
var extraBackends []string
