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
