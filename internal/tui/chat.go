package tui

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/KazChe/cs/internal/store"
)

// chatTextMsg carries a chunk of assistant text streamed from `claude -p`.
type chatTextMsg struct{ text string }

// chatToolMsg carries a one-line note about a tool the model ran (or a tool
// error), shown inline in the transcript.
type chatToolMsg struct{ text string }

// chatDoneMsg ends a turn; sid is the session id claude reported (captured on the
// first turn so later turns can --resume it), err is any process error.
type chatDoneMsg struct {
	sid string
	err error
}

// openChat switches into the per-customer chat pane, loading (or preparing to
// create) that account's persistent Claude session.
func (m *Model) openChat(c customer) tea.Cmd {
	m.mode = modeChat
	m.chatCust = c
	m.chatLog = []string{fmt.Sprintf(
		"Chat with %s (%s). Ask about this account, or ask me to update it (changes are committed to history). Enter to send, esc to go back.",
		c.name, c.id)}
	m.chatCur = ""
	m.streaming = false
	if m.sub == nil {
		m.sub = make(chan tea.Msg, 256)
	}
	ensureChatTable(m.st)
	m.chatSID = lookupSession(m.st, c.id)

	m.input.Reset()
	m.input.Focus()
	m.resizeChat()
	m.syncViewport()
	return textinputBlink
}

// csBinary returns the absolute, symlink-resolved path to this running cs
// binary. The chat pane scopes the model's Bash allowlist to exactly this path
// (rather than the bare name `cs`), so only our binary is runnable even if some
// other `cs` sits on PATH. Falls back to "cs" if the path cannot be resolved,
// so the pane still works rather than blocking every command.
func csBinary() string {
	exe, err := os.Executable()
	if err != nil {
		return "cs"
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	if abs, err := filepath.Abs(exe); err == nil {
		exe = abs
	}
	return exe
}

// startTurn fires one chat turn: builds fresh account context plus the command
// catalog, launches claude in a goroutine that streams events onto m.sub, and
// starts listening for them.
func (m *Model) startTurn(prompt string) tea.Cmd {
	bin := csBinary()
	sys := buildContext(m.st, m.chatCust.id) + "\n" + toolInstructions(m.chatCust.id, bin)
	sid := m.chatSID
	sub := m.sub
	repo := m.st.Dir
	launch := func() tea.Msg {
		runClaude(sub, sid, sys, prompt, repo, bin)
		return nil
	}
	return tea.Batch(launch, listen(sub))
}

func listen(sub chan tea.Msg) tea.Cmd {
	return func() tea.Msg { return <-sub }
}

// runClaude invokes `claude -p` in headless streaming mode with the Bash tool
// scoped to the absolute cs binary path, and forwards assistant text, tool
// activity, and a final done signal onto sub. The prompt goes via stdin because
// --allowedTools is variadic and would otherwise swallow a positional prompt.
// CS_DIR is baked into the child env so any cs command the model runs hits the
// same repo.
func runClaude(sub chan tea.Msg, sid, sysPrompt, prompt, repoDir, bin string) {
	go func() {
		args := []string{"-p", "--output-format", "stream-json", "--verbose",
			"--allowedTools", "Bash(" + bin + ":*)"}
		if sid != "" {
			args = append(args, "--resume", sid)
		}
		if sysPrompt != "" {
			args = append(args, "--append-system-prompt", sysPrompt)
		}

		cmd := exec.Command("claude", args...)
		cmd.Stdin = strings.NewReader(prompt)
		cmd.Env = append(os.Environ(), "CS_DIR="+repoDir)
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			sub <- chatDoneMsg{err: err}
			return
		}
		if err := cmd.Start(); err != nil {
			sub <- chatDoneMsg{err: err}
			return
		}

		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
		var newSID string
		for sc.Scan() {
			var ev map[string]any
			if json.Unmarshal(sc.Bytes(), &ev) != nil {
				continue
			}
			if s, ok := ev["session_id"].(string); ok && s != "" {
				newSID = s
			}
			switch ev["type"] {
			case "assistant":
				for _, msg := range assistantBlocks(ev) {
					sub <- msg
				}
			case "user":
				if e := toolResultError(ev); e != "" {
					sub <- chatToolMsg{text: "✗ " + e}
				}
			}
		}
		werr := cmd.Wait()
		sub <- chatDoneMsg{sid: newSID, err: werr}
	}()
}

// assistantBlocks turns one assistant event into ordered messages: text chunks
// and a one-line note per Bash command the model invoked.
func assistantBlocks(ev map[string]any) []tea.Msg {
	msg, _ := ev["message"].(map[string]any)
	content, _ := msg["content"].([]any)
	var out []tea.Msg
	for _, c := range content {
		cm, _ := c.(map[string]any)
		switch cm["type"] {
		case "text":
			if t := str(cm["text"]); t != "" {
				out = append(out, chatTextMsg{t})
			}
		case "tool_use":
			inp, _ := cm["input"].(map[string]any)
			if cmdStr := str(inp["command"]); cmdStr != "" {
				out = append(out, chatToolMsg{text: "⚙ " + cmdStr})
			}
		}
	}
	return out
}

func toolResultError(ev map[string]any) string {
	msg, _ := ev["message"].(map[string]any)
	content, _ := msg["content"].([]any)
	for _, c := range content {
		cm, _ := c.(map[string]any)
		if cm["type"] == "tool_result" {
			if b, _ := cm["is_error"].(bool); b {
				return firstLine(str(cm["content"]))
			}
		}
	}
	return ""
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 160 {
		s = s[:160]
	}
	return s
}

// toolInstructions is the command catalog handed to the model: an explicit,
// enforced list of what it may run, mirroring a tool-selection contract. bin is
// the absolute cs binary path the Bash allowlist is scoped to; the model must
// invoke exactly that path or the command is denied, so the catalog uses it.
func toolInstructions(custID, bin string) string {
	return "You can act on this account by running cs commands with the Bash tool. " +
		"Only run the cs binary at " + bin + "; invoke it by that exact absolute path, not the bare name `cs`. " +
		"The data repo is preselected via CS_DIR, so never pass --repo. " +
		"Always pass --commit on changes so they persist to history. " +
		"Target account for any action: " + custID + ". " +
		"After acting, confirm briefly what you changed. In every command below, " +
		"replace `cs` with " + bin + ":\n" +
		"  cs note -c <id> -k call|slack|email|ticket|meeting|note \"<summary>\" --commit\n" +
		"  cs item add -c <id> -t bug|feature|question|action -p <1-3> \"<title>\" --commit\n" +
		"  cs item resolve <item-id> --commit\n" +
		"  cs item ls -c <id>\n" +
		"  cs stage <id> <to-stage> --reason \"<why>\" --commit\n" +
		"  cs link <from-item> <to-item> --rel blocks|relates|raised_in|advances_stage|supersedes --commit\n" +
		"  cs show <id>\n" +
		"  cs week <id>\n"
}

// buildContext renders the account's current state as a plain-text system prompt
// so the model answers from live data, re-sent each turn since the board changes
// under a resumed session.
func buildContext(st *store.Store, id string) string {
	var b strings.Builder
	b.WriteString("You are helping a customer-success engineer with one account. Use only the data below; do not invent facts. Be concise.\n\n")

	head, _ := st.Query("SELECT name,stage,health FROM customers WHERE id=" + q(id))
	if len(head) > 0 {
		h := head[0]
		b.WriteString(fmt.Sprintf("Account: %v (%s)  stage=%v  health=%v\n", h["name"], id, h["stage"], h["health"]))
	}

	b.WriteString("\nOpen items:\n")
	items, _ := st.Query("SELECT id,type,priority,title,created_at FROM items WHERE customer_id=" +
		q(id) + " AND status<>'resolved' ORDER BY created_at")
	if len(items) == 0 {
		b.WriteString("  (none)\n")
	}
	for _, r := range items {
		b.WriteString(fmt.Sprintf("  %v p%v %v %v (%s old)\n",
			str(r["id"]), str(r["priority"]), str(r["type"]), str(r["title"]), ageDays(r["created_at"])))
	}

	b.WriteString("\nRecent activity:\n")
	acts, _ := st.Query("SELECT kind,summary,occurred_at FROM activities WHERE customer_id=" +
		q(id) + " ORDER BY occurred_at DESC LIMIT 8")
	if len(acts) == 0 {
		b.WriteString("  (none)\n")
	}
	for _, r := range acts {
		b.WriteString(fmt.Sprintf("  [%v] %v (%s ago)\n", str(r["kind"]), str(r["summary"]), ageDays(r["occurred_at"])))
	}

	b.WriteString("\nTrajectory:\n")
	stages, _ := st.Query("SELECT from_stage,to_stage,reason,occurred_at FROM stage_events WHERE customer_id=" +
		q(id) + " ORDER BY occurred_at")
	if len(stages) == 0 {
		b.WriteString("  (no recorded transitions)\n")
	}
	for _, r := range stages {
		reason := ""
		if s := str(r["reason"]); s != "" && s != "<nil>" {
			reason = " (" + s + ")"
		}
		b.WriteString(fmt.Sprintf("  %v -> %v%s\n", str(r["from_stage"]), str(r["to_stage"]), reason))
	}
	return b.String()
}

func ensureChatTable(st *store.Store) {
	_ = st.Exec("CREATE TABLE IF NOT EXISTS chat_sessions (" +
		"customer_id VARCHAR(64) PRIMARY KEY, session_id VARCHAR(64) NOT NULL, created_at DATETIME)")
}

func lookupSession(st *store.Store, custID string) string {
	rows, err := st.Query("SELECT session_id FROM chat_sessions WHERE customer_id=" + q(custID))
	if err != nil || len(rows) == 0 {
		return ""
	}
	return str(rows[0]["session_id"])
}

func saveSession(st *store.Store, custID, sid string) {
	_ = st.Exec(fmt.Sprintf(
		"INSERT INTO chat_sessions (customer_id,session_id,created_at) VALUES (%s,%s,NOW())",
		q(custID), q(sid)))
}

// resizeChat sizes the transcript viewport and input to the current window.
func (m *Model) resizeChat() {
	w := m.width
	if w < 20 {
		w = 80
	}
	h := m.height - 6
	if h < 3 {
		h = 10
	}
	m.vp.Width = w
	m.vp.Height = h
	m.input.Width = w - 4
}

func (m *Model) syncViewport() {
	lines := append([]string{}, m.chatLog...)
	if m.chatCur != "" {
		lines = append(lines, "cs: "+m.chatCur)
	}
	w := m.vp.Width
	if w < 20 {
		w = 60
	}
	body := lipgloss.NewStyle().Width(w).Render(strings.Join(lines, "\n\n"))
	m.vp.SetContent(body)
	m.vp.GotoBottom()
}

func (m Model) chatView() string {
	status := ""
	if m.streaming {
		status = "  · thinking…"
	}
	title := titleStyle.Render(fmt.Sprintf("Chat · %s (%s)%s", m.chatCust.name, m.chatCust.id, status))
	foot := footerStyle.Render("enter send · esc back · ctrl+c quit")
	return title + "\n" + m.vp.View() + "\n" + m.input.View() + "\n" + foot + "\n"
}
