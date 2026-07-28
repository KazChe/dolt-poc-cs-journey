//go:build !embedded

package store

import "fmt"

// This stub stands in for the embedded backend in default (non-embedded) builds.
// The real dolthub/driver/v2 implementation lives in backend_embedded.go behind
// the `embedded` build tag, so the plain `cs` binary never pulls the CGO/ICU
// dependency. Requesting CS_STORE_BACKEND=embedded without an embedded-tagged
// build fails fast with an actionable message rather than silently falling back.

type embeddedUnavailable struct{}

func newEmbeddedBackend(string) backend { return embeddedUnavailable{} }

func (embeddedUnavailable) err() error {
	return fmt.Errorf("%s=%s requires a build with -tags embedded; this binary was built without the embedded driver",
		EnvBackend, backendEmbedded)
}

func (e embeddedUnavailable) ensureInit() error                      { return e.err() }
func (e embeddedUnavailable) exec(string) error                      { return e.err() }
func (e embeddedUnavailable) execScript(string) error                { return e.err() }
func (e embeddedUnavailable) query(string) ([]map[string]any, error) { return nil, e.err() }
func (e embeddedUnavailable) commit(string) error                    { return e.err() }
