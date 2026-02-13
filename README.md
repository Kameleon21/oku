# Oku

[![Contributors](https://img.shields.io/github/contributors/Kameleon21/oku)](https://github.com/Kameleon21/oku/graphs/contributors)

Terminal companion for [Hardcover](https://hardcover.app): browse your shelves, search books, and update progress without leaving the terminal.

![Oku demo flow](./oku-demo.gif)

## What You Get

- CLI for scripting and quick actions
- Full TUI dashboard (`oku` or `oku tui`)
- Vim-first navigation in TUI (arrow keys also work)
- Search modes: `book`, `author`, `genre`
- Output density modes: `compact`, `default`, `verbose`
- Local SQLite cache with auto-refresh and manual sync

## Requirements

- Go (project currently uses `go 1.25.7`)
- Hardcover API token
- Keychain support (recommended) or `HARDCOVER_TOKEN` env var

## Install

```bash
go install github.com/Kameleon21/oku/cmd/oku@latest
```

If you are installing from an untagged branch:

```bash
go install github.com/Kameleon21/oku/cmd/oku@main
```

## Quick Start

```bash
go build -o oku ./cmd/oku

# Save token in keychain
./oku auth set-token

# Pull data to local cache
./oku sync

# Launch dashboard
./oku
```

Token lookup order:

1. `HARDCOVER_TOKEN`
2. Keychain (`service=oku`, `account=hardcover`)

## Core Commands

```text
oku auth set-token
oku config show
oku config edit

oku list <reading|oku|finished|dnf> [--refresh]
oku reading|oku|finished|dnf [--refresh]
oku now [--refresh]
oku sync

oku search <query> [--mode book|author|genre] [--limit N]
oku status <reading|oku|finished|dnf> [--book ID]
oku update --page <N|+N|-N> [--book ID]

oku active
oku set-active --book ID
oku open

oku tui
```

Global flags:

- `--json` return JSON output
- `--view compact|default|verbose` control CLI/TUI output density

Exit codes:

- `0` success
- `1` validation/app error
- `2` network/transient API error

## TUI (LazyGit-Style Flow)

Run with `oku` (interactive terminal) or `oku tui`.

Layout:

- Left: `Reading`, `Oku`, and search input
- Right: selected book details or search results

Key controls:

### Global

- `h` / `l` or `←` / `→`: move between panes
- `j` / `k`: move in lists
- `?`: help
- `q`: quit

### Library Panes

- `Enter`: toggle selected book between `Reading` and `Oku`
- `+` / `-`: quick page update (`+10` / `-10`)
- `u`: custom page update
- `g` / `w` / `f` / `d`: set status (reading / want / finished / dnf)
- `x`: remove from lists
- `z`: cycle density (compact/default/verbose)
- `r`: refresh
- `s`: sync all
- `/`: focus search input

### Search Input

- `i` or `a`: enter insert mode
- `Esc`: insert -> normal (no forced pane jump)
- `Enter`: run search
- `m`: cycle mode (`book -> author -> genre`)
- `1` / `2` / `3`: set mode directly
- In normal mode, `h/l` navigates panes (does not type)

### Search Results

- `Enter`: add selected result to reading
- `g` / `w` / `f` / `d`: add result with selected status
- `Esc`: back to search input

## Data Shown

CLI/TUI details include:

- title, authors, progress, pages
- rating and rating counts
- review/user popularity counts
- release date
- slug and IDs in verbose views

Search supports intent-based discovery:

- `book`: title-weighted
- `author`: author-weighted
- `genre`: genre/tag-weighted

## Cache + API Behavior

- Cache-first reads from SQLite
- Auto-refresh if cache is empty or stale (6h)
- `--refresh` forces API refresh for list commands
- `oku sync` refreshes all statuses
- API client uses timeout, retry, and throttling (~1 req/sec)

## Config + Data Paths

- Config: `~/.config/oku/config.toml` (or `$XDG_CONFIG_HOME/oku/config.toml`)
- Data: `~/.local/share/oku/` (or `$XDG_DATA_HOME/oku/`)
- DB file: `~/.local/share/oku/oku.db` (XDG equivalent)

Example config:

```toml
editor = "nvim"
use_fzf = false
default_list = "reading"
```

## Development

```bash
go test ./...
go build ./cmd/oku
```

## Contributors

[![Contributors](https://contrib.rocks/image?repo=Kameleon21/oku)](https://github.com/Kameleon21/oku/graphs/contributors)
