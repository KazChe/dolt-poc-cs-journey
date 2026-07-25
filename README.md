# cs

A small CLI that tracks customer-success work as a versioned graph, so a
customer's journey is queryable through time. Activities, stateful asks,
relationships, and stage transitions live in a [Dolt](https://www.dolthub.com)
repo. Because Dolt is git-for-data, you get `diff` and point-in-time (`AS OF`)
history for free.

Status: early and evolving. The command surface is intentionally small.

## How it works

`cs` shells out to the `dolt` CLI to read and write a Dolt repo (your data), and
renders the results for you or for an AI agent. The data repo is separate from
this code repo, so your notes never live in here.

## Prerequisites

- **Go 1.26+** (to build the binary)
- **Dolt** (the database). Install it from the official guide:
  <https://www.dolthub.com/docs/introduction/installation/>
  On macOS that is `brew install dolt`. Verify with `dolt version`.

## Install

```bash
git clone https://github.com/KazChe/cs.git
cd cs
go build -o cs .
# optionally put it on your PATH:
# mv cs /usr/local/bin/
```

## Setup

1. Tell Dolt who you are, once, like git, so commits have an author:

   ```bash
   dolt config --global --add user.name "Your Name"
   dolt config --global --add user.email "you@example.com"
   ```

2. Create the data repo that will hold your customer journeys:

   ```bash
   mkdir cs-journey && cd cs-journey
   dolt init
   ```

3. Point `cs` at it and create the tables:

   ```bash
   export CS_DIR="$PWD"      # or pass --repo <path> to any command
   cs init
   ```

## Usage

A full loop on one account: adding it, updating it, and reading it back. Each
mutating command takes `--commit` to snapshot the change into Dolt history; omit
it to stage several edits and commit them yourself later. Point-in-time commands
like `cs week` read that history, so commit the steps you want to see there.

```bash
# Add an account (new accounts start in the onboarding stage).
cs customer add acme "Acme Corp" --commit

# Journal an activity (a call, in this case).
cs note -c acme -k call "kickoff call, flagged a slow dashboard" --commit

# Add a stateful item to track. Its generated id is printed, e.g. itm-jt4rb3.
cs item add -c acme -t bug -p 1 "Dashboard loads slowly for admins" --commit

# Advance the journey as onboarding wraps up.
cs stage acme adoption --reason "onboarding complete" --commit

# See what is still open for the account.
cs item ls -c acme

# Update the item: resolve it once the fix ships.
cs item resolve itm-jt4rb3 --commit     # or run `cs item resolve` to fuzzy-pick

# Read the whole trajectory back: state, open items, recent activity, stages.
cs show acme

# What changed on items in the last 7 days (reads Dolt history).
cs week acme

# A bounded snapshot of every account (also what the Claude Code hook injects).
cs prime
```

Most commands take a customer or item id directly, or open an interactive fuzzy
picker when you omit it (`cs note`, `cs item resolve`, `cs link`), which needs a
real terminal. Dependencies between items are recorded with `cs link <from> <to>
--rel blocks`. Run `cs <command> --help` for every flag.

`cs board` opens a live TUI that lays accounts out as a "parade" across their
journey stages (onboarding through renewed), one card per customer colored by
health. It re-queries every few seconds, so it reflects edits made from other
terminals. Arrow keys (or `hjkl`) move the selection, `enter` drills into a
customer detail view (open items, recent activity, trajectory), `esc` returns,
`r` refreshes now, and `q` quits. When there are more lanes than fit on screen,
it scrolls horizontally to keep the selected lane in view. Needs a real
terminal.

Press `c` on a selected card to open a **chat pane** for that account. It shells
out to Claude Code in headless mode (`claude -p`) and streams the answer into the
pane, with the account's current state (open items, recent activity, trajectory)
injected as context each turn. Each customer gets its own persistent Claude
session, stored in a `chat_sessions` table and resumed on the next chat, so the
conversation continues where it left off.

The chat can also **act on the account** when you ask it to: it is given the
Bash tool scoped to `cs` commands only, so it can resolve items, log notes,
change stage, and so on. Commands it runs are shown inline in the transcript
(`⚙ cs item resolve ...`), it uses `--commit` so every change lands in Dolt
history (auditable and revertible with `dolt log` / `dolt revert`), and it
cannot run anything other than `cs`. It only acts when you ask; plain questions
stay read-only. `esc` returns to the board.

This needs the `claude` and `cs` CLIs on your PATH and uses your Claude Code
authentication (and tokens).

## Claude Code hook

`cs setup claude` registers a SessionStart hook so every Claude Code session
opens with the account snapshot from `cs prime`.

```bash
cs setup claude                 # install into ./.claude/settings.json (this project)
cs setup claude --global        # install into ~/.claude/settings.json (all projects)
cs setup claude --remove        # remove it again
```

The absolute paths to the `cs` binary and the resolved Dolt repo are baked into
the hook command, so it fires correctly no matter which project directory Claude
Code opens in. Re-run the command after moving the binary or the data repo.

## Configuration

- `--repo <path>` or the `CS_DIR` environment variable selects the Dolt data
  repo. It defaults to the current directory.

## Notes

- `cs` currently drives Dolt by shelling out to the `dolt` binary. It can later
  embed Dolt in-process via `dolthub/driver/v2` without changing any command
  code, since every command is just SQL.

## License

MIT. See [LICENSE](LICENSE). Use it, fork it, build on it.
