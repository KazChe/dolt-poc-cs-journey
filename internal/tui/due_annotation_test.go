package tui

import (
	"testing"
	"time"
)

func TestDueAnnotation(t *testing.T) {
	day := func(off int) string { return time.Now().AddDate(0, 0, off).Format("2006-01-02") }

	tests := []struct {
		name        string
		in          any
		wantText    string
		wantOverdue bool
	}{
		{"nil is empty", nil, "", false},
		{"empty string is empty", "", "", false},
		{"garbage is empty", "soon", "", false},
		{"today counts as due not overdue", day(0), "due " + day(0), false},
		{"future is due", day(5), "due " + day(5), false},
		{"past is overdue", day(-3), "⚠ overdue " + day(-3), true},
		{"datetime form parses", time.Now().AddDate(0, 0, 2).Format("2006-01-02") + " 00:00:00", "due " + day(2), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text, overdue := dueAnnotation(tt.in)
			if text != tt.wantText || overdue != tt.wantOverdue {
				t.Errorf("dueAnnotation(%v) = (%q,%v), want (%q,%v)", tt.in, text, overdue, tt.wantText, tt.wantOverdue)
			}
		})
	}
}
