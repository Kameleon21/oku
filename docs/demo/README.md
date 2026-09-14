# Record the README demo

From the repository root, with Go, Python 3, [VHS](https://github.com/charmbracelet/vhs), ttyd, and ffmpeg installed:

```sh
sh docs/demo/record.sh
```

The script builds the current checkout into a temporary directory, creates a disposable SQLite library with fictional books, and enables Oku's sample statistics. A placeholder token overrides the system keychain. The recording starts inside the TUI: it browses a book, opens the theme picker with `T`, previews and applies Nord, visits the shelf and statistics, then shows Solarized Light and returns to Catppuccin Mocha. Theme changes happen live inside the app; setup commands and the shell are hidden. It does not search, sync, or update Hardcover. Theme selections are saved to the temporary configuration, never to yours.

The temporary configuration, database, and executable are removed on exit. Outputs are `oku-demo.gif` (displayed in the README) and `oku-demo.mp4` (an ignored local preview). Edit `oku-demo.tape` to change timing and navigation. The recording enables true color and clears `NO_COLOR` only in its own terminal so the themes remain visible regardless of the parent shell's settings.

The tape uses numbered tabs and the in-app theme picker. `TestDemoTapeKeysStillNavigate` in `internal/tui` replays its dashboard keys, checking the selected tab, modal, palette, and saved theme. A keymap change that sends the recording somewhere else fails the build rather than the next recording.
