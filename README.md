# Oku

[![Contributors](https://img.shields.io/github/contributors/Kameleon21/oku)](https://github.com/Kameleon21/oku/graphs/contributors)

Terminal companion for [Hardcover](https://hardcover.app): browse your shelves, search books, and track reading — all without leaving the terminal.

![Oku demo flow](./oku-demo.gif)

## Features

- Full TUI dashboard with vim-style navigation (`h/j/k/l`)
- CLI for scripting and quick actions
- Search by book, author, or genre
- Reading streaks and stats tracking
- Reading timer to log sessions
- Local SQLite cache with auto-refresh

## Install

Stable release:

```bash
go install github.com/Kameleon21/oku/cmd/oku@latest
```

Development branch (latest features):

```bash
go install github.com/Kameleon21/oku/cmd/oku@develop
```

## Getting Started

```bash
# Save your Hardcover API token
oku auth set-token

# Pull your library to local cache
oku sync

# Launch the TUI
oku
```

Your token is stored in the system keychain. You can also set `HARDCOVER_TOKEN` as an environment variable.

## Usage

```text
oku                    Launch TUI dashboard
oku tui                Launch TUI dashboard

oku search <query>     Search books (--mode book|author|genre)
oku now                Show current read
oku reading            Show reading list
oku update --page <N>  Update page progress

oku sync               Refresh all cached data
oku auth set-token     Set API token
oku config show        Show configuration
```

Use `--json` for JSON output and `--view compact|default|verbose` to control detail level.

## TUI Navigation

Oku uses vim-style keybindings throughout. Arrow keys also work.

| Key | Action |
|-----|--------|
| `h/l` | Move between panes |
| `j/k` | Navigate lists |
| `/` | Search |
| `Enter` | Select / toggle status |
| `+/-` | Quick page update |
| `?` | Help |
| `q` | Quit |

## Config

Config lives at `~/.config/oku/config.toml`:

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
