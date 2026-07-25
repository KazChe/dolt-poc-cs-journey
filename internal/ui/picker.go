// Package ui holds interactive selection helpers built on go-fuzzyfinder.
package ui

import (
	"fmt"

	"github.com/ktr0731/go-fuzzyfinder"

	"github.com/KazChe/cs/internal/store"
)

// PickCustomer shows a fuzzy finder over customers and returns the chosen id.
// The preview pane shows the highlighted customer's stage and health.
func PickCustomer(st *store.Store) (string, error) {
	rows, err := st.Query("SELECT id, name, stage, health FROM customers ORDER BY name")
	if err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "", fmt.Errorf("no customers yet: add one with `cs customer add <id> <name>`")
	}
	idx, err := fuzzyfinder.Find(
		rows,
		func(i int) string {
			return fmt.Sprintf("%v  %v", rows[i]["id"], rows[i]["name"])
		},
		fuzzyfinder.WithPreviewWindow(func(i, _, _ int) string {
			if i < 0 {
				return ""
			}
			r := rows[i]
			return fmt.Sprintf("%v\n\nstage:  %v\nhealth: %v", r["name"], r["stage"], r["health"])
		}),
	)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%v", rows[idx]["id"]), nil
}

// PickItem shows a fuzzy finder over items matching whereClause (a SQL
// condition) and returns the chosen id. The preview shows the item's detail.
func PickItem(st *store.Store, whereClause string) (string, error) {
	rows, err := st.Query(
		"SELECT id, customer_id, type, status, priority, title FROM items WHERE " + whereClause + " ORDER BY created_at")
	if err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "", fmt.Errorf("no matching items")
	}
	idx, err := fuzzyfinder.Find(
		rows,
		func(i int) string {
			r := rows[i]
			return fmt.Sprintf("%v  %v  %v", r["customer_id"], r["id"], r["title"])
		},
		fuzzyfinder.WithPreviewWindow(func(i, _, _ int) string {
			if i < 0 {
				return ""
			}
			r := rows[i]
			return fmt.Sprintf("%v\n\ncustomer: %v\ntype:     %v\nstatus:   %v\npriority: %v",
				r["title"], r["customer_id"], r["type"], r["status"], r["priority"])
		}),
	)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%v", rows[idx]["id"]), nil
}
