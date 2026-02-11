# Oku

A terminal companion for [Hardcover](https://hardcover.app). Track what you're reading, update your progress, and manage your shelves -- all without leaving the terminal.

Oku gives you two ways to work:

- **CLI** -- scriptable commands for quick actions and automation
- **TUI dashboard** -- a full interactive view that launches by default

## What's Working

Everything you need for day-to-day use is in place:

- All CLI commands
- Interactive TUI dashboard (`oku` or `oku tui`)
- Local SQLite cache with stale-refresh and pruning
- Hardcover API integration with auth, throttling, timeouts, and retries

## Getting Started

### Prerequisites

- Go (`go 1.25.7` or compatible)
- A [Hardcover](https://hardcover.app) API token
- macOS keychain support (via `go-keyring`), or set `HARDCOVER_TOKEN` in your environment

### Build and Run

```bash
go build -o oku ./cmd/oku

# Store your token in the system keychain
./oku auth set-token

# Pull your library into the local cache
./oku sync

# Open the dashboard
./oku
```

Prefer environment variables? That works too:

```bash
export HARDCOVER_TOKEN="your_token"
```

Token lookup order:
1. `HARDCOVER_TOKEN` environment variable
2. System keychain (`service=oku`, `account=hardcover`)

## Commands

```text
oku active                                    Show active books
oku auth set-token                            Store your API token
oku config edit                               Open config in your editor
oku config show                               Print current config
oku list <reading|oku|finished|dnf> [--refresh]
oku now [--refresh]                           What are you reading right now?
oku search <query> [--limit N]                Search Hardcover
oku set-active --book <id>                    Add a book to your active list
oku open                                      Pick a currently-reading book and add it to active list
oku update --page <N|+N|-N> [--book <id>]     Update page progress
oku status <reading|oku|finished|dnf> [--book <id>]
oku sync                                      Refresh the local cache
oku tui                                       Launch the dashboard
```

There are shortcuts for listing shelves:

```bash
oku reading    # currently reading
oku oku        # want to read
oku finished   # done
oku dnf        # did not finish
```

Add `--json` to any CLI command for machine-readable output.

Exit codes: `0` success, `1` user/app error, `2` network/transient failure.

## TUI Dashboard

Just run `oku` in a terminal (or `oku tui` explicitly).

The dashboard has **Reading** and **Oku** lists stacked vertically on the left, with a full-height **Output** panel on the right showing book details. Press `?` at any time to open the help modal with all keybindings.

### Keybindings

**Library view:**

| Key | Action |
|-----|--------|
| `Tab` | Switch between Reading and Oku |
| `/` | Search Hardcover |
| `Enter` | Toggle selected book between Reading and Oku |
| `u` | Update page progress |
| `g` `w` `f` `d` | Move to Reading / Oku / Finished / DNF |
| `x` | Remove book from library lists |
| `r` | Refresh from API |
| `s` | Sync all statuses |
| `?` | Show help modal |
| `q` | Quit |

**Search mode:**

| Key | Action |
|-----|--------|
| Type + `Enter` | Search |
| `Enter` on result | Add to Reading |
| `g` `w` `f` `d` | Assign status to result |
| `?` | Show help modal |
| `Esc` | Back to library |

## How Caching Works

Oku tries to be a good API citizen. It caches aggressively and only hits Hardcover when it needs to:

- **Cache-first** for all status lists
- **Auto-refresh** when the cache is empty or older than **6 hours**
- **Manual refresh** with `--refresh` on any list command, or `oku sync` for everything
- **Clean replacement** on sync -- no stale leftovers
- **Orphan pruning** -- cached books not referenced by any list are cleaned up after 30 days

Under the hood, the API client talks to `https://api.hardcover.app/v1/graphql` with a ~1 req/sec throttle, 30s timeout, and automatic retries on transient errors (`429`, `5xx`, deadline exceeded). User cancellations are respected and not retried.

## Configuration

Config lives at `~/.config/oku/config.toml` (or `$XDG_CONFIG_HOME/oku/config.toml`).

```toml
editor = "nvim"
use_fzf = false
default_list = "reading"
```

Data and cache are stored under `~/.local/share/oku/` (or `$XDG_DATA_HOME/oku/`).

## Architecture

For anyone poking around the code:

```
cmd/oku/          Entrypoint
internal/
  cli/            Cobra commands + TUI
  app/            Business logic, orchestration
  api/            Hardcover GraphQL client
  store/          SQLite cache and state
  auth/           Token resolution + keychain
  config/         Config loading and XDG paths
  model/          Domain types, statuses, page parsing
  picker/         Interactive list picker (fzf-style)
```

The flow is straightforward: CLI/TUI -> `app` -> `store` + `api`. The app layer owns the logic, the store owns the cache, and the API client handles Hardcover.

## Development

```bash
go test ./...
go vet ./...
go build ./cmd/oku
```

Smoke test checklist:

1. `oku auth set-token`
2. `oku sync`
3. `oku search "east of eden"`
4. `oku tui`
5. `oku update --page 50`
6. Check your progress on [hardcover.app](https://hardcover.app) to confirm it synced
