package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var (
	setupGlobal bool
	setupRemove bool
	setupRC     string
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Wire cs into an agent (Claude Code)",
}

var setupClaudeCmd = &cobra.Command{
	Use:   "claude",
	Short: "Install a SessionStart hook that primes Claude Code with the account snapshot",
	Long: "Registers `cs prime --hook-json` as a Claude Code SessionStart hook so every\n" +
		"session opens with a bounded snapshot of your accounts. The current cs binary\n" +
		"and Dolt repo paths are baked into the hook, so it works from any project\n" +
		"directory. Re-run after moving the binary or the data repo.",
	RunE: func(cmd *cobra.Command, args []string) error {
		settingsPath, err := claudeSettingsPath(setupGlobal)
		if err != nil {
			return err
		}

		settings, err := readSettings(settingsPath)
		if err != nil {
			return err
		}

		hooks, ok := settings["hooks"].(map[string]any)
		if !ok {
			hooks = map[string]any{}
			settings["hooks"] = hooks
		}

		command, err := hookCommand()
		if err != nil {
			return err
		}

		var hookChanged bool
		if setupRemove {
			removeHook(hooks, "SessionStart", command)
			if len(hooks) == 0 {
				delete(settings, "hooks")
			}
			hookChanged = true
		} else {
			hookChanged = addHook(hooks, "SessionStart", command)
		}

		if hookChanged {
			if err := writeSettings(settingsPath, settings); err != nil {
				return err
			}
		}

		// Manage the CLAUDE.md pointer that tells agents to use cs read
		// commands for account depth. It sits next to the project settings
		// (skipped for --global, which has no single project directory).
		var pointerChanged bool
		if !setupGlobal {
			mdPath := claudeMdPath(settingsPath)
			var err error
			if setupRemove {
				pointerChanged, err = removePointer(mdPath)
			} else {
				pointerChanged, err = addPointer(mdPath)
			}
			if err != nil {
				return err
			}
		}

		if setupRemove {
			if !hookChanged && !pointerChanged {
				fmt.Printf("✓ nothing to remove for %s\n", settingsPath)
				return nil
			}
			fmt.Printf("✓ removed cs SessionStart hook from %s\n", settingsPath)
			if pointerChanged {
				fmt.Println("  removed the account-depth pointer from CLAUDE.md")
			}
			return nil
		}

		if !hookChanged && !pointerChanged {
			fmt.Printf("✓ hook and CLAUDE.md pointer already present in %s\n", settingsPath)
			return nil
		}
		if hookChanged {
			fmt.Printf("✓ installed cs SessionStart hook in %s\n", settingsPath)
			fmt.Printf("  command: %s\n", command)
		} else {
			fmt.Printf("✓ hook already present in %s\n", settingsPath)
		}
		if pointerChanged {
			fmt.Println("  added the account-depth pointer to CLAUDE.md")
		}
		fmt.Println("  restart Claude Code for it to take effect.")
		return nil
	},
}

var setupShellCmd = &cobra.Command{
	Use:   "shell",
	Short: "Add a quiet due-check to your shell startup file (CLI-side session start)",
	Long: "Appends a small block to your shell rc (~/.zshrc by default) that runs\n" +
		"`cs due --quiet` when a new terminal opens. It prints one compact line when\n" +
		"something is overdue or due soon, and nothing at all otherwise — the CLI\n" +
		"equivalent of the Claude Code SessionStart hook. Re-run after moving the\n" +
		"binary or repo; use --remove to take it out. --rc <path> targets a specific\n" +
		"file (e.g. ~/.bashrc).",
	RunE: func(cmd *cobra.Command, args []string) error {
		rcPath, err := shellRCPath(setupRC)
		if err != nil {
			return err
		}

		if setupRemove {
			changed, err := removeShellSnippet(rcPath)
			if err != nil {
				return err
			}
			if !changed {
				fmt.Printf("✓ nothing to remove in %s\n", rcPath)
				return nil
			}
			fmt.Printf("✓ removed cs due snippet from %s\n", rcPath)
			return nil
		}

		snippet, err := shellSnippet()
		if err != nil {
			return err
		}
		changed, err := addShellSnippet(rcPath, snippet)
		if err != nil {
			return err
		}
		if !changed {
			fmt.Printf("✓ cs due snippet already present in %s\n", rcPath)
			return nil
		}
		fmt.Printf("✓ added cs due snippet to %s\n", rcPath)
		fmt.Println("  open a new terminal (or source the file) for it to take effect.")
		return nil
	},
}

// shellSnippetStart / shellSnippetEnd fence the block cs manages in the rc, so
// setup can add or remove exactly it without disturbing the rest of the file.
const (
	shellSnippetStart = "# cs:due:start"
	shellSnippetEnd   = "# cs:due:end"
)

// shellRCPath resolves the rc file to edit: an explicit --rc path, else
// ~/.zshrc (the macOS default login shell).
func shellRCPath(explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".zshrc"), nil
}

// shellSnippet builds the fenced rc block that runs the quiet due check, baking
// in absolute paths to this binary and the resolved repo so it fires from any
// directory a new shell starts in.
func shellSnippet() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if abs, err := filepath.Abs(exe); err == nil {
		exe = abs
	}
	cmd := shellQuote(exe) + " due --quiet"
	// Bake in the resolved repo (absolute) so the snippet fires from any startup
	// directory. resolveRepoDir always returns a path now (~/.cs by default).
	{
		repo := resolveRepoDir()
		if abs, err := filepath.Abs(repo); err == nil {
			repo = abs
		}
		cmd += " --repo " + shellQuote(repo)
	}
	// Startup code must never be noisy. Guard on the binary existing (so a stale
	// rc line after a move/uninstall is a no-op, not "command not found"), and
	// swallow stderr plus any non-zero exit (`|| true`) so a transient failure —
	// dolt missing from PATH, an unreadable repo — fails silent rather than
	// dumping an error into every new terminal.
	return fmt.Sprintf("%s\n[ -x %s ] && { %s ; } 2>/dev/null || true\n%s\n",
		shellSnippetStart, shellQuote(exe), cmd, shellSnippetEnd), nil
}

// addShellSnippet appends the snippet to the rc file unless an equivalent block
// is already present. Returns false when nothing changed.
func addShellSnippet(path, snippet string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	existing := string(data)
	if strings.Contains(existing, shellSnippetStart) {
		return false, nil
	}
	var b strings.Builder
	if len(strings.TrimSpace(existing)) > 0 {
		b.WriteString(strings.TrimRight(existing, "\n"))
		b.WriteString("\n\n")
	}
	b.WriteString(snippet)
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return false, err
	}
	return true, nil
}

// removeShellSnippet strips the fenced cs due block from the rc file. Returns
// false when no block was present.
func removeShellSnippet(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	existing := string(data)
	start := strings.Index(existing, shellSnippetStart)
	if start < 0 {
		return false, nil
	}
	end := strings.Index(existing, shellSnippetEnd)
	if end < 0 {
		return false, nil
	}
	end += len(shellSnippetEnd)
	remaining := strings.TrimSpace(existing[:start] + existing[end:])
	if remaining == "" {
		if err := os.Remove(path); err != nil {
			return false, err
		}
		return true, nil
	}
	if err := os.WriteFile(path, []byte(remaining+"\n"), 0o644); err != nil {
		return false, err
	}
	return true, nil
}

// pointerBlock is the CLAUDE.md section telling agents to use cs read commands
// for account depth. It is fenced by markers so setup can add or remove exactly
// this block without disturbing anything else in the file. Keep it in sync with
// the CLAUDE.md checked into the repo root.
const (
	pointerStart = "<!-- cs:account-depth:start -->"
	pointerEnd   = "<!-- cs:account-depth:end -->"
	pointerBlock = pointerStart + `
## Account depth: use ` + "`cs`" + `, not external connectors

This project tracks customer-success work in a local Dolt repo through the ` + "`cs`" + `
CLI. The SessionStart hook (` + "`cs prime`" + `) injects a bounded snapshot of every
account (stage, health, open-item count). That summary is intentionally shallow.

When you need detail on a specific account, run the ` + "`cs`" + ` read commands rather
than reaching for Gmail, Drive, Calendar, or other external sources. The ` + "`cs`" + `
data is the source of truth for account state:

- ` + "`cs show <id>`" + ` reads the whole trajectory: current stage, health, open items,
  recent activity.
- ` + "`cs week <id>`" + ` shows what changed on the account's items in the last 7 days
  (backed by Dolt history).
- ` + "`cs item ls -c <id>`" + ` lists the account's open items.

Replace ` + "`<id>`" + ` with the account id shown in the snapshot (for example ` + "`acme`" + `).
These commands are read-only and safe to run. Run ` + "`cs <command> --help`" + ` for
flags.
` + pointerEnd + "\n"
)

// claudeMdPath returns the CLAUDE.md path sitting beside a project settings
// file (../CLAUDE.md relative to the .claude/settings.json path).
func claudeMdPath(settingsPath string) string {
	dir := filepath.Dir(filepath.Dir(settingsPath))
	return filepath.Join(dir, "CLAUDE.md")
}

// addPointer ensures the pointer block is present in the CLAUDE.md at path,
// creating the file if needed and appending the block otherwise. Returns false
// when the block was already present.
func addPointer(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	existing := string(data)
	if strings.Contains(existing, pointerStart) {
		return false, nil
	}
	var b strings.Builder
	if len(strings.TrimSpace(existing)) > 0 {
		b.WriteString(strings.TrimRight(existing, "\n"))
		b.WriteString("\n\n")
	}
	b.WriteString(pointerBlock)
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return false, err
	}
	return true, nil
}

// removePointer strips the pointer block from the CLAUDE.md at path. If that
// leaves the file empty, the file is removed. Returns false when no block was
// present.
func removePointer(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	existing := string(data)
	start := strings.Index(existing, pointerStart)
	if start < 0 {
		return false, nil
	}
	end := strings.Index(existing, pointerEnd)
	if end < 0 {
		return false, nil
	}
	end += len(pointerEnd)
	remaining := strings.TrimSpace(existing[:start] + existing[end:])
	if remaining == "" {
		if err := os.Remove(path); err != nil {
			return false, err
		}
		return true, nil
	}
	if err := os.WriteFile(path, []byte(remaining+"\n"), 0o644); err != nil {
		return false, err
	}
	return true, nil
}

// claudeSettingsPath returns the project (.claude/settings.json under cwd) or
// global (~/.claude/settings.json) settings path.
func claudeSettingsPath(global bool) (string, error) {
	if global {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".claude", "settings.json"), nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Join(wd, ".claude", "settings.json"), nil
}

// hookCommand builds the shell command the hook runs, baking in absolute paths
// to this binary and the resolved Dolt repo so it fires correctly no matter
// which directory Claude Code opens in.
func hookCommand() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if abs, err := filepath.Abs(exe); err == nil {
		exe = abs
	}
	cmd := shellQuote(exe) + " prime --hook-json"
	// Bake in the resolved repo (absolute) so the hook fires from any project
	// directory. resolveRepoDir always returns a path now (~/.cs by default).
	{
		repo := resolveRepoDir()
		if abs, err := filepath.Abs(repo); err == nil {
			repo = abs
		}
		cmd += " --repo " + shellQuote(repo)
	}
	return cmd, nil
}

// shellQuote single-quotes a string for a POSIX shell, escaping embedded quotes.
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	if !strings.ContainsAny(s, " \t'\"\\$`") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func readSettings(path string) (map[string]any, error) {
	settings := map[string]any{}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return settings, nil
		}
		return nil, err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return settings, nil
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return settings, nil
}

func writeSettings(path string, settings map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// addHook appends a command hook for the given event unless it is already
// present. Returns false when nothing changed.
func addHook(hooks map[string]any, event, command string) bool {
	entries, _ := hooks[event].([]any)
	for _, e := range entries {
		em, ok := e.(map[string]any)
		if !ok {
			continue
		}
		cmds, _ := em["hooks"].([]any)
		for _, c := range cmds {
			cm, ok := c.(map[string]any)
			if ok && cm["command"] == command {
				return false
			}
		}
	}
	entries = append(entries, map[string]any{
		"matcher": "",
		"hooks": []any{
			map[string]any{"type": "command", "command": command},
		},
	})
	hooks[event] = entries
	return true
}

// removeHook drops any command hook matching command from the event, pruning
// empty entries and the event key itself when nothing is left.
func removeHook(hooks map[string]any, event, command string) {
	entries, ok := hooks[event].([]any)
	if !ok {
		return
	}
	kept := make([]any, 0, len(entries))
	for _, e := range entries {
		em, ok := e.(map[string]any)
		if !ok {
			kept = append(kept, e)
			continue
		}
		cmds, _ := em["hooks"].([]any)
		remaining := make([]any, 0, len(cmds))
		for _, c := range cmds {
			cm, ok := c.(map[string]any)
			if ok && cm["command"] == command {
				continue
			}
			remaining = append(remaining, c)
		}
		if len(remaining) == 0 {
			continue
		}
		em["hooks"] = remaining
		kept = append(kept, em)
	}
	if len(kept) == 0 {
		delete(hooks, event)
		return
	}
	hooks[event] = kept
}

func init() {
	setupClaudeCmd.Flags().BoolVar(&setupGlobal, "global", false, "write to ~/.claude/settings.json instead of ./.claude/settings.json")
	setupClaudeCmd.Flags().BoolVar(&setupRemove, "remove", false, "remove the hook instead of installing it")

	setupShellCmd.Flags().StringVar(&setupRC, "rc", "", "shell rc file to edit (default ~/.zshrc)")
	setupShellCmd.Flags().BoolVar(&setupRemove, "remove", false, "remove the snippet instead of installing it")

	setupCmd.AddCommand(setupClaudeCmd, setupShellCmd)
	rootCmd.AddCommand(setupCmd)
}
