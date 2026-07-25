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

```bash
cs customer add acme "Acme Corp" --stage adoption   # add an account
cs note                                             # fuzzy-pick a customer, then type the note
cs note -c acme "cadence call, discussed roadmap"   # or name the customer directly
cs prime                                            # a bounded snapshot of every account
cs prime --hook-json                                # same, wrapped for a Claude Code SessionStart hook
```

Running `cs note` with no `-c` opens an interactive fuzzy picker, which needs a
real terminal.

## Configuration

- `--repo <path>` or the `CS_DIR` environment variable selects the Dolt data
  repo. It defaults to the current directory.

## Notes

- `cs` currently drives Dolt by shelling out to the `dolt` binary. It can later
  embed Dolt in-process via `dolthub/driver/v2` without changing any command
  code, since every command is just SQL.
- No license file is included yet. Add one before sharing this publicly.
