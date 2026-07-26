package cmd

import "testing"

// resetItemLsFlags clears the package-level `item ls` flag vars so each test
// case starts from a known state.
func resetItemLsFlags() {
	itemStatus = ""
	itemAll = false
	itemResolved = false
}

func TestItemStatusFilter(t *testing.T) {
	tests := []struct {
		name     string
		status   string
		all      bool
		resolved bool
		want     string
		wantErr  bool
	}{
		{name: "default is open only", want: "status <> 'resolved'"},
		{name: "resolved only", resolved: true, want: "status='resolved'"},
		{name: "all has no status predicate", all: true, want: ""},
		{name: "exact status", status: "open", want: "status='open'"},
		{name: "status conflicts with all", status: "open", all: true, wantErr: true},
		{name: "status conflicts with resolved", status: "open", resolved: true, wantErr: true},
		{name: "resolved conflicts with all", resolved: true, all: true, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetItemLsFlags()
			defer resetItemLsFlags()
			itemStatus, itemAll, itemResolved = tt.status, tt.all, tt.resolved

			got, err := itemStatusFilter()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("itemStatusFilter() = %q, want error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("itemStatusFilter() unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("itemStatusFilter() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestItemLsEmptyMsg(t *testing.T) {
	tests := []struct {
		name     string
		status   string
		all      bool
		resolved bool
		want     string
	}{
		{name: "default", want: "(no open items)"},
		{name: "resolved", resolved: true, want: "(no resolved items)"},
		{name: "all", all: true, want: "(no items)"},
		{name: "exact status", status: "waiting", want: `(no items with status "waiting")`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetItemLsFlags()
			defer resetItemLsFlags()
			itemStatus, itemAll, itemResolved = tt.status, tt.all, tt.resolved

			if got := itemLsEmptyMsg(); got != tt.want {
				t.Fatalf("itemLsEmptyMsg() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFmtDay(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want string
	}{
		{name: "dolt datetime", in: "2026-07-26 14:03:05", want: "2026-07-26"},
		{name: "rfc3339", in: "2026-07-26T14:03:05Z", want: "2026-07-26"},
		{name: "null resolved_at (nil)", in: nil, want: ""},
		{name: "empty string", in: "", want: ""},
		{name: "unparseable", in: "not-a-date", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fmtDay(tt.in); got != tt.want {
				t.Fatalf("fmtDay(%v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
