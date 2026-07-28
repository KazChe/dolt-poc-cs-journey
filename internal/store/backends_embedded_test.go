//go:build embedded

package store

// Embedded-tagged builds add the embedded backend to the parity suite, so every
// behavior test asserts shellout and embedded are observably identical.
var extraBackends = []string{backendEmbedded}
