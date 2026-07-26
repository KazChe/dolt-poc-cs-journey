package cmd

import "testing"

// day returns a YYYY-MM-DD string offset from today, for date-relative cases
// that stay correct no matter when the suite runs. Uses the same date
// normalization as production (todayDate) so the test's notion of "today"
// matches the code under test.
func day(offset int) string {
	return todayDate().AddDate(0, 0, offset).Format("2006-01-02")
}

func TestDueLiteral(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{name: "empty clears to NULL", in: "", want: "NULL"},
		{name: "valid date quoted", in: "2026-08-15", want: "'2026-08-15'"},
		{name: "wrong format rejected", in: "08-15-2026", wantErr: true},
		{name: "garbage rejected", in: "soon", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := dueLiteral(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("dueLiteral(%q) = %q, want error", tt.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("dueLiteral(%q) unexpected error: %v", tt.in, err)
			}
			if got != tt.want {
				t.Fatalf("dueLiteral(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestDueAnnotation(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want string
	}{
		{name: "null is empty", in: nil, want: ""},
		{name: "empty string is empty", in: "", want: ""},
		{name: "unparseable is empty", in: "nope", want: ""},
		{name: "past date is overdue", in: day(-3), want: "⚠ overdue " + day(-3)},
		{name: "today is due (not overdue)", in: day(0), want: "due " + day(0)},
		{name: "future date is due", in: day(5), want: "due " + day(5)},
		{name: "dolt datetime form", in: day(-1) + " 00:00:00", want: "⚠ overdue " + day(-1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := dueAnnotation(tt.in); got != tt.want {
				t.Fatalf("dueAnnotation(%v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestSplitDue(t *testing.T) {
	rows := []map[string]any{
		{"due_at": day(-2)}, // overdue
		{"due_at": day(0)},  // today -> upcoming
		{"due_at": day(4)},  // upcoming
		{"due_at": nil},     // skipped
	}
	overdue, upcoming := splitDue(rows)
	if len(overdue) != 1 {
		t.Fatalf("overdue = %d, want 1", len(overdue))
	}
	if len(upcoming) != 2 {
		t.Fatalf("upcoming = %d, want 2", len(upcoming))
	}
}

func TestDueQuietLine(t *testing.T) {
	one := []map[string]any{{"due_at": day(0)}}
	two := []map[string]any{{"due_at": day(0)}, {"due_at": day(1)}}
	tests := []struct {
		name     string
		overdue  []map[string]any
		upcoming []map[string]any
		want     string
	}{
		{name: "nothing is silent", want: ""},
		{name: "overdue only", overdue: one, want: "⏰ 1 overdue — cs due"},
		{name: "upcoming only", upcoming: one, want: "⏰ 1 due soon — cs due"},
		{name: "both", overdue: one, upcoming: two, want: "⏰ 1 overdue · 2 due soon — cs due"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := dueQuietLine(tt.overdue, tt.upcoming); got != tt.want {
				t.Fatalf("dueQuietLine() = %q, want %q", got, tt.want)
			}
		})
	}
}
