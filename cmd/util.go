package cmd

import "strings"

// sqlStr quotes and escapes a string literal for the Dolt CLI. This tool drives
// dolt via string SQL, so at minimum single quotes are doubled. It is a local
// single-user tool, not a server, but escaping is still the floor.
func sqlStr(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
