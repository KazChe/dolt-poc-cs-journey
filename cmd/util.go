package cmd

import (
	"bufio"
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/KazChe/cs/internal/store"
	"github.com/KazChe/cs/internal/ui"
)

// promptLine prints label and reads a single line from stdin. A trailing EOF
// with data (e.g. piped input without a newline) still returns that data.
func promptLine(label string) (string, error) {
	fmt.Print(label)
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return line, nil
}

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

// parseDay parses a dolt DATE/TIMESTAMP value into a calendar date (a
// midnight-UTC time.Time carrying only year/month/day), and reports whether it
// was parseable and non-null. Normalizing to a pure date keeps comparisons at
// day granularity and free of timezone-offset skew — never use Truncate here,
// since it aligns to UTC midnight regardless of the value's location.
func parseDay(v any) (time.Time, bool) {
	s, ok := v.(string)
	if !ok || s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{"2006-01-02", "2006-01-02 15:04:05", time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return dateOnly(t), true
		}
	}
	return time.Time{}, false
}

// dateOnly strips the time-of-day, yielding midnight UTC on the same calendar
// date. todayDate is the same normalization applied to the local current date.
func dateOnly(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func todayDate() time.Time {
	return dateOnly(time.Now())
}

// dueAnnotation renders an item's due_at as a compact status suffix relative to
// today: "⚠ overdue <date>" when the date has passed, "due <date>" otherwise
// (a date equal to today counts as due, not overdue). Empty for a null/unset
// due date so callers can omit it entirely.
func dueAnnotation(v any) string {
	d, ok := parseDay(v)
	if !ok {
		return ""
	}
	if d.Before(todayDate()) {
		return "⚠ overdue " + d.Format("2006-01-02")
	}
	return "due " + d.Format("2006-01-02")
}

// asInt coerces a value from a dolt JSON result into an int. Dolt returns
// numeric columns as JSON numbers (float64), but tolerate a stringified form
// too. Returns 0 for anything unparseable or null.
func asInt(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case string:
		var i int
		fmt.Sscanf(n, "%d", &i)
		return i
	}
	return 0
}

// fmtDay renders a timestamp value as a calendar date, e.g. "2026-07-26".
// Empty if the value is null or unparseable (e.g. an unresolved item's
// resolved_at).
func fmtDay(v any) string {
	s, ok := v.(string)
	if !ok || s == "" {
		return ""
	}
	t, err := time.Parse("2006-01-02 15:04:05", s)
	if err != nil {
		if t, err = time.Parse(time.RFC3339, s); err != nil {
			return ""
		}
	}
	return t.Format("2006-01-02")
}
