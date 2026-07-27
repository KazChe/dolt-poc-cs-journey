package tui

import (
	"fmt"
	"strings"
	"testing"
)

// TestDetailRenderSmoke exercises the detail render path headlessly: it builds a
// Model with sample rows, sizes it, focuses each pane, and asserts the body is
// non-empty and contains the selection marker. Also prints the view under -v so a
// human can eyeball the look.
func TestDetailRenderSmoke(t *testing.T) {
	m := Model{}
	m.width, m.height = 100, 30
	m.mode = modeDetail
	m.detailVP.Width, m.detailVP.Height = 100, 26
	m.detail = detail{
		c: customer{id: "globex", name: "Globex Corp", stage: "renewal_risk", health: "red"},
		items: []map[string]any{
			{"id": "itm-h2dml3", "priority": "1", "type": "bug", "title": "Onboarding blocker: SSO loops", "created_at": "2026-07-01 10:00:00"},
			{"id": "itm-a9x", "priority": "2", "type": "feature", "title": "Bulk export", "created_at": "2026-07-10 10:00:00"},
			{"id": "itm-zz2", "priority": "3", "type": "question", "title": "SLA clarification", "created_at": "2026-07-20 10:00:00"},
		},
		acts: []map[string]any{
			{"kind": "call", "summary": "kickoff call, flagged slow dashboard", "occurred_at": "2026-07-22 09:00:00"},
			{"kind": "email", "summary": "sent renewal proposal", "occurred_at": "2026-07-24 09:00:00"},
		},
		stages: []map[string]any{
			{"from_stage": "onboarding", "to_stage": "adoption", "reason": "onboarding complete", "occurred_at": "2026-06-01 09:00:00"},
			{"from_stage": "adoption", "to_stage": "renewal_risk", "reason": "usage dropped", "occurred_at": "2026-07-15 09:00:00"},
		},
	}

	for _, f := range []detailPane{paneItems, paneActivity, paneTrajectory} {
		m.detailFocus = f
		m.detailItem = 1
		m.syncDetail()
		out := m.detailView()
		if strings.TrimSpace(out) == "" {
			t.Fatalf("empty detail view for pane %d", f)
		}
		if !strings.Contains(out, "Globex Corp") {
			t.Errorf("header missing customer name (pane %d)", f)
		}
	}

	// Selection marker present when Items focused.
	m.detailFocus = paneItems
	m.detailItem = 0
	m.syncDetail()
	if !strings.Contains(m.detailBody(), "▸") {
		t.Errorf("selection marker missing in items pane")
	}

	if testing.Verbose() {
		m.detailFocus = paneItems
		m.detailItem = 0
		m.syncDetail()
		fmt.Println("\n--- detailView (Items focused) ---")
		fmt.Println(m.detailView())
	}
}
