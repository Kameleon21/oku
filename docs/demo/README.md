# Record the README demo

From the repository root, with Go, Python 3, [VHS](https://github.com/charmbracelet/vhs), ttyd, and ffmpeg installed:

```sh
sh docs/demo/record.sh
```

The script builds the current checkout into a temporary directory, creates a disposable SQLite library with fictional books, and enables Oku's sample statistics. The seeded reads started days ago and there is a books goal for the year, so the forecasts have a pace to show; the finished books match the sample statistics, so every number on screen agrees. A placeholder token overrides the system keychain. The recording starts inside the TUI in the `oku` theme: the preview pane forecasts each book's finish, Stats shows the goal's pace and the reads finishing next, then the theme picker (`T`) previews and applies `oku-light` and Nord before returning to `oku`. Theme changes happen live inside the app; setup commands and the shell are hidden. It does not search, sync, or update Hardcover, and it does not open the detail pane, which would fetch the journal. Theme selections are saved to the temporary configuration, never to yours.

The launch refresh still runs against the placeholder token and fails; the tape waits, hidden, until that error toast has cleared.

The temporary configuration, database, and executable are removed on exit. Outputs are `oku-demo.gif` (displayed in the README) and `oku-demo.mp4` (an ignored local preview). Edit `oku-demo.tape` to change timing and navigation. The recording enables true color and clears `NO_COLOR` only in its own terminal so the themes remain visible regardless of the parent shell's settings.

The tape uses numbered tabs and the in-app theme picker. `TestDemoTapeKeysStillNavigate` in `internal/tui` replays its dashboard keys, checking the selected tab, modal, palette, and saved theme. A keymap change that sends the recording somewhere else fails the build rather than the next recording.
