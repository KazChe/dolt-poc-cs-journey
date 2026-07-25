package cmd

import (
	"crypto/rand"
	"fmt"
	"strings"
	"time"

	"github.com/KazChe/cs/internal/store"
	"github.com/KazChe/cs/internal/ui"
)

// sqlStr quotes and escapes a string literal for the Dolt CLI. This tool drives
// dolt via string SQL, so at minimum single quotes are doubled. It is a local
// single-user tool, not a server, but escaping is still the floor.
func sqlStr(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

const idAlphabet = "0123456789abcdefghijklmnopqrstuvwxyz"

// newID returns a short, typeable id like "itm-k3f9a1". 6 base36 chars is ~2
// billion values, ample for a single-user tool, and the fuzzy picker means you
// rarely type one anyway.
func newID(prefix string) string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%s-%06x", prefix, time.Now().UnixNano()&0xffffff)
	}
	for i := range b {
		b[i] = idAlphabet[int(b[i])%len(idAlphabet)]
	}
	return prefix + "-" + string(b)
}

// resolveCustomer returns explicit if set, otherwise opens the fuzzy picker.
func resolveCustomer(st *store.Store, explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	return ui.PickCustomer(st)
}

// maybeCommit commits the working set when do is true.
func maybeCommit(st *store.Store, do bool, msg string) error {
	if !do {
		return nil
	}
	if err := st.Commit(msg); err != nil {
		return err
	}
	fmt.Println("✓ committed")
	return nil
}

// ageDays renders how old a timestamp value is, e.g. "6d". Empty if unparseable.
func ageDays(v any) string {
	s, ok := v.(string)
	if !ok {
		return ""
	}
	t, err := time.Parse("2006-01-02 15:04:05", s)
	if err != nil {
		if t, err = time.Parse(time.RFC3339, s); err != nil {
			return ""
		}
	}
	return fmt.Sprintf("%dd", int(time.Since(t).Hours()/24))
}
