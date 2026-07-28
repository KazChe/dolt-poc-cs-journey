# Embedded Dolt Driver vs. Shell-out: Benchmark & Findings

Research notes for issue #4 — replacing the per-call `dolt` process spawn in
`internal/store` with the in-process `dolthub/driver/v2` driver.

## TL;DR

Running Dolt SQL in-process instead of spawning the `dolt` binary per call is
roughly **25x faster for writes** and **~1000x faster for reads**, with
identical on-disk data and behavior. The embedded path is gated behind both a
build tag and a runtime env flag so the default `cs` binary stays pure-Go with
no new toolchain requirements.

## Background

`internal/store` is the only component that talks to Dolt. Before this work,
every store operation (`EnsureInit`, `Exec`, `ExecScript`, `Query`, `Commit`)
shelled out to the `dolt` CLI via `exec.Command("dolt", ...)`. That means each
call pays a full OS process spawn plus Dolt's own startup cost, even for a
one-row read.

The hypothesis: because `database/sql` + `dolthub/driver/v2` keep a long-lived
in-process connection, embedding the driver amortizes that startup once instead
of paying it per call. The benchmarks below put real numbers on it.

## Method

- Machine: Apple M1 Pro (`darwin/arm64`)
- Dolt CLI: 2.2.2 · driver: `github.com/dolthub/driver/v2 v2.2.0` (version-aligned)
- Both backends exercised by the **same** table-driven tests and benchmarks in
  `internal/store`, selected via the `CS_STORE_BACKEND` env var.
- `go test -tags embedded -run '^$' -bench 'BenchmarkExec|BenchmarkQuery' -benchtime 30x ./internal/store/`

## Results

Per-op latency, 30 iterations each:

| Operation | Shell-out | Embedded | Speedup |
|-----------|-----------|----------|---------|
| `Exec`    | ~127.7 ms/op | ~5.16 ms/op | ~25x |
| `Query`   | ~121.0 ms/op | ~0.12 ms/op | ~1000x |

Raw output:

```
goos: darwin
goarch: arm64
pkg: github.com/KazChe/cs/internal/store
cpu: Apple M1 Pro
BenchmarkExec/shellout-10         30   127692293 ns/op
BenchmarkExec/embedded-10         30     5160494 ns/op
BenchmarkQuery/shellout-10        30   121043335 ns/op
BenchmarkQuery/embedded-10        30      119971 ns/op
```

`Query` shows the largest gap: shell-out pays a full `dolt` process spawn on
every read, while the embedded path reuses one in-process connection. `Exec`
still involves a write/commit path, so the absolute time is higher, but the
per-call spawn tax disappears there too.

## What we did

1. **Wrote an impl-agnostic test harness first** (`store_test.go`, merged in
   PR #26) against the current shell-out impl — 6 behavior tests covering init
   idempotency, script + query round-trip, empty results, error surfacing, and
   commit + `dolt_log` history. Because these tests assert behavior, not
   implementation, they double as the parity net for the swap.

2. **Introduced a `backend` seam** in `internal/store`. The public `Store` type
   and every caller signature (`Query/Exec/ExecScript/Commit/EnsureInit`) stay
   untouched; underneath sits an unexported `backend` interface with two impls:
   - `shellBackend` — the existing shell-out code, moved verbatim.
   - `embeddedBackend` — new, `dolthub/driver/v2` in-process.

3. **Runtime selection** via `CS_STORE_BACKEND` (`shellout` | `embedded`).
   Default is `shellout`, so nothing changes for existing users until the flag
   is flipped. An unset or unrecognized value falls back to shellout.

4. **Ran the same 6 tests against both backends** (12 subtests) — all pass
   identically, proving parity.

## Findings worth remembering

- **The harness caught a real bug mid-build.** `database/sql` pools
  connections, so an initial `USE db` issued on one connection did not carry to
  the next pooled connection, which came back "no database selected." Fixed by
  binding the database in the DSN so every connection defaults to it. Without
  the parity tests this would have shipped silently.

- **`dolt init` still shells out.** Repo creation is not a driver operation, so
  `EnsureInit` continues to invoke `dolt init` for the one-time bootstrap. Only
  the hot-path operations run in-process.

- **The embedded driver pulls a heavy transitive dependency.**
  `dolthub/go-icu-regex` requires **CGO + an ICU C++ toolchain** to compile. To
  keep the default `cs` binary pure-Go with zero new build burden, the entire
  embedded path lives behind a `//go:build embedded` tag:
  - `go build ./...` → pure-Go, shellout only, no CGO. Unchanged for everyone.
  - `go build -tags embedded ./...` (with ICU flags) → embedded backend
    available, selectable via `CS_STORE_BACKEND=embedded`.

  A non-embedded build that requests `CS_STORE_BACKEND=embedded` fails fast with
  an actionable message (via `backend_embedded_stub.go`) rather than silently
  falling back.

- **The gate is both build-time and runtime.** Build-time keeps the default
  binary clean; runtime lets an embedded-tagged build flip between backends
  without recompiling.

## Reproducing the embedded build (macOS + Homebrew ICU)

```sh
ICU=$(brew --prefix icu4c@78)
export CGO_ENABLED=1
export CGO_CPPFLAGS="-I$ICU/include"
export CGO_LDFLAGS="-L$ICU/lib"
export PKG_CONFIG_PATH="$ICU/lib/pkgconfig"

# both backends, all tests
go test -tags embedded ./internal/store/

# benchmarks
go test -tags embedded -run '^$' -bench 'BenchmarkExec|BenchmarkQuery' \
  -benchtime 30x ./internal/store/
```

## Recommendation

Gate for one release (default `shellout`) to prove parity and performance in
the wild, then flip the default to `embedded` and eventually retire the
shell-out path so we are not carrying two backends indefinitely.
