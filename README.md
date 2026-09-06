# Oku

<p align="center">
  <img src="docs/assets/mascot-page-spirit.png" width="220" alt="Oku mascot: an ivory origami page spirit with vermilion folds">
</p>

[![Release](https://img.shields.io/github/v/release/Kameleon21/oku?style=flat)](https://github.com/Kameleon21/oku/releases/latest)
[![Stars](https://img.shields.io/github/stars/Kameleon21/oku?style=flat)](https://github.com/Kameleon21/oku/stargazers)
[![Contributors](https://img.shields.io/github/contributors/Kameleon21/oku?style=flat)](https://github.com/Kameleon21/oku/graphs/contributors)

Your [Hardcover](https://hardcover.app) library in your terminal. Browse your shelves, find your next book, and track your reading with a keyboard-driven dashboard or quick CLI commands.

![Oku dashboard, sample reading shelves, statistics, and timer picker](oku-demo.gif)

*The recording uses fictional books and sample reading data. [Recording instructions](docs/demo/README.md).*

## Install and run

On macOS, install with Homebrew:

```sh
brew tap Kameleon21/oku
brew install --cask oku
```

For Linux and Windows, download and extract a prebuilt archive from
[GitHub Releases](https://github.com/Kameleon21/oku/releases/latest) and add the
`oku` binary (`oku.exe` on Windows) to your `PATH`.

Alternatively, install from source with **Go 1.25.7 or later**:

```sh
go install github.com/Kameleon21/oku/cmd/oku@latest
```

Make sure Go's binary directory (usually `~/go/bin`) is on your `PATH`.
Run `oku --version` to check your version.

### Connect Hardcover

Create an account at [hardcover.app](https://hardcover.app), then copy your API
token from [Account Settings](https://hardcover.app/account/api).

```sh
oku auth set-token  # Save your Hardcover API token
oku sync            # Pull your library into the local cache
oku                 # Launch the dashboard
```

Your token is stored in the system keychain. You can also set `HARDCOVER_TOKEN`,
which takes priority over the saved token.

## Everyday use

Browse reading lists, search by book, author, or genre, and update your progress
without leaving the terminal. The dashboard also shows your Hardcover reading
goal, yearly summary, activity heatmap, ratings, and genre breakdowns.

```sh
oku reading                          # Currently reading
oku finished                         # Finished books
oku search "Ursula K. Le Guin" --mode author
oku update --book 123 --page +10      # Add 10 pages to a book's progress
oku stats                            # Reading stats and activity heatmap
oku sync                             # Refresh cached Hardcover data
```

Replace `123` with a book ID from your library. You can omit `--book` when there
is exactly one active book. `--page` accepts an absolute page number or a relative
change such as `+10` or `-5`.

Use `--json` on commands that support structured output, such as
`oku reading --json`. Set `--view compact|default|verbose` to adjust output
density. Run `oku --help` or `oku <command> --help` for all commands and flags.

### Dashboard controls

Launch with `oku` or `oku tui`. Navigation uses vim-style keys; arrow keys also work.

| Key | What it does |
| --- | --- |
| `h` / `l` | Move between panes |
| `j` / `k` | Navigate lists |
| `/` | Search |
| `Enter` | Open details |
| `+` / `-` | Quick page update |
| `U` | Undo the last change while its toast is visible |
| `?` / `q` | Help / quit |

Press `?` for all controls available in the focused section.

### Reading timer

Track time spent reading with local sessions:

```sh
oku timer start     # Choose a currently reading book
oku timer status    # Check elapsed time
oku timer stop      # Save the session
oku timer stats     # Review reading time
```

Timer sessions are stored locally, separately from your Hardcover reading stats.

## Configuration

Run `oku config edit` to open your settings, or `oku config show` to see the
configuration and data paths. On macOS and Linux, the default configuration file
is `~/.config/oku/config.toml`; on Windows it is `%AppData%\oku\config.toml`.
`XDG_CONFIG_HOME` overrides the configuration directory. Existing Windows
installations may continue using the legacy `~/.config/oku/config.toml` file.

```toml
editor = "nvim"
use_fzf = false
default_list = "reading"
theme = "auto" # auto | dark | light
```

The dashboard and colored CLI output adapt to light and dark terminals.
Set `theme` explicitly if your terminal reports its background incorrectly.
`NO_COLOR` is supported; borders and a `▸` marker also indicate focus.

Oku caches library data in SQLite and refreshes it automatically. Run `oku sync`
for a full refresh, or use `oku reading --refresh` to refresh a reading list.

## Contributing

Use the Go version specified in [go.mod](go.mod), then run:

```sh
go test ./...
go vet ./...
go build ./cmd/oku
```

Open feature pull requests against `develop`. See the
[development and release guide](docs/development.md) for the branch workflow and
release steps. To try the development version:

```sh
go install github.com/Kameleon21/oku/cmd/oku@develop
```

[![Contributors](https://contrib.rocks/image?repo=Kameleon21/oku)](https://github.com/Kameleon21/oku/graphs/contributors)

## License

[MIT](LICENSE).
