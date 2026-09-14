Output oku-demo.gif
Output oku-demo.mp4
Set FontSize 18
Set Width 1280
Set Height 800
Set Theme "Catppuccin Mocha"
Set TypingSpeed 90ms
Set Padding 16
Set Framerate 15
Env NO_COLOR ""
Env COLORTERM "truecolor"
Env CLICOLOR "1"

# Start inside the app. Setup and the shell are outside the recording.
Hide
Type "oku"
Enter
Sleep 2s
Show
Sleep 2.5s

# Browse a book and its detail pane.
Type "j"
Sleep 1.5s
Enter
Sleep 2s
Escape

# Open the in-app theme picker and preview Nord live, then apply it.
Type "T"
Sleep 1.5s
Type "nord"
Sleep 2.5s
Enter
Sleep 2s

# The want-to-read shelf and reading statistics, still in Nord.
Type "2"
Sleep 2s
Type "4"
Sleep 3s

# Show a light background changing live inside the TUI.
Type "T"
Sleep 1s
Type "solarized-light"
Sleep 2.5s
Enter
Sleep 2s

# Return to Reading and restore Mocha through the same picker.
Type "1"
Sleep 1.5s
Type "T"
Type "mocha"
Sleep 1.5s
Enter
Sleep 2.5s

# End on the opening view for a quiet loop; keep the shell out of frame.
Type "k"
Sleep 1.5s
Hide
Type "q"
Sleep 500ms
