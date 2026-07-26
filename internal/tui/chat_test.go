package tui

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestCSBinaryIsAbsolute checks that csBinary resolves to an absolute path,
// which is what the Bash allowlist is scoped to.
func TestCSBinaryIsAbsolute(t *testing.T) {
	bin := csBinary()
	if bin == "" {
		t.Fatal("csBinary returned empty string")
	}
	// The fallback is the bare name "cs"; any other result must be absolute.
	if bin != "cs" && !filepath.IsAbs(bin) {
		t.Fatalf("csBinary = %q, want an absolute path or the \"cs\" fallback", bin)
	}
}

// TestToolInstructionsScopedToBinary checks that the command catalog tells the
// model to invoke the exact binary the allowlist permits. If the catalog used
// the bare name while the allowlist is scoped to an absolute path, every command
// would be denied.
func TestToolInstructionsScopedToBinary(t *testing.T) {
	const bin = "/opt/cs/cs"
	got := toolInstructions("acme", bin)
	if !strings.Contains(got, bin) {
		t.Fatalf("toolInstructions does not mention the scoped binary %q:\n%s", bin, got)
	}
	if !strings.Contains(got, "acme") {
		t.Fatalf("toolInstructions dropped the target account id:\n%s", got)
	}
}
