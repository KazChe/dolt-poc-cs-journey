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

		if setupRemove {
			removeHook(hooks, "SessionStart", command)
			if len(hooks) == 0 {
				delete(settings, "hooks")
			}
		} else if !addHook(hooks, "SessionStart", command) {
			fmt.Printf("✓ hook already present in %s\n", settingsPath)
			return nil
		}

		if err := writeSettings(settingsPath, settings); err != nil {
			return err
		}

		if setupRemove {
			fmt.Printf("✓ removed cs SessionStart hook from %s\n", settingsPath)
			return nil
		}
		fmt.Printf("✓ installed cs SessionStart hook in %s\n", settingsPath)
		fmt.Printf("  command: %s\n", command)
		fmt.Println("  restart Claude Code for it to take effect.")
		return nil
	},
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
	if repo := resolveRepoDir(); repo != "" {
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
	setupCmd.AddCommand(setupClaudeCmd)
	rootCmd.AddCommand(setupCmd)
}
