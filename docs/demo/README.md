# Record the README demo

From the repository root, with Go, Python 3, [VHS](https://github.com/charmbracelet/vhs), ttyd, and ffmpeg installed:

```sh
sh docs/demo/record.sh
```

The script builds the current checkout into a temporary directory, creates a disposable SQLite library with fictional books, and enables Oku's sample statistics. A placeholder token overrides the system keychain. The tape previews the colour themes, picks Catppuccin Mocha, browses local shelves, opens a book in the detail pane, visits the statistics and the timer picker, and opens the help modal; then it switches to Nord and walks the tabs once more. It does not search, sync, or update Hardcover. The `theme` it writes goes to the temporary configuration, never to yours.

The temporary configuration, database, and executable are removed on exit. Outputs are `oku-demo.gif` (displayed in the README) and `oku-demo.mp4` (an ignored local preview). Edit `oku-demo.tape` to change timing and navigation.

The tape moves between tabs with `l` and `h`, so its keys follow the header strip: Reading, Oku, Search, Stats, Timer. `TestDemoTapeKeysStillNavigate` in `internal/tui` replays each dashboard session of the tape (every `oku` up to its `q`) against the dashboard, so a keymap change that would send the recording somewhere else fails the build rather than the next recording. What is typed at the shell between sessions is not replayed.
