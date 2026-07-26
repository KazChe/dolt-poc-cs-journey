<!-- cs:account-depth:start -->
## Account depth: use `cs`, not external connectors

This project tracks customer-success work in a local Dolt repo through the `cs`
CLI. The SessionStart hook (`cs prime`) injects a bounded snapshot of every
account (stage, health, open-item count). That summary is intentionally shallow.

When you need detail on a specific account, run the `cs` read commands rather
than reaching for Gmail, Drive, Calendar, or other external sources. The `cs`
data is the source of truth for account state:

- `cs show <id>` reads the whole trajectory: current stage, health, open items,
  recent activity.
- `cs week <id>` shows what changed on the account's items in the last 7 days
  (backed by Dolt history).
- `cs item ls -c <id>` lists the account's open items.

Replace `<id>` with the account id shown in the snapshot (for example `acme`).
These commands are read-only and safe to run. Run `cs <command> --help` for
flags.
<!-- cs:account-depth:end -->
