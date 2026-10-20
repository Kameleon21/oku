Output oku-demo.gif
Output oku-demo.mp4
Set FontSize 18
Set Width 1280
Set Height 800
# The terminal around the dashboard in the oku palette, so the frame and
# the app read as one surface while oku is applied.
Set Theme { "name": "oku", "background": "#1A1917", "foreground": "#EEE7D8", "cursor": "#E5533C", "selection": "#3B3732", "black": "#25231F", "red": "#E5533C", "green": "#93AD78", "yellow": "#E0A93E", "blue": "#E3B47C", "magenta": "#EC6F86", "cyan": "#C8BEAB", "white": "#EEE7D8", "brightBlack": "#91877A", "brightRed": "#F39470", "brightGreen": "#93AD78", "brightYellow": "#E0A93E", "brightBlue": "#E3B47C", "brightMagenta": "#EC6F86", "brightCyan": "#C8BEAB", "brightWhite": "#FFFFFF" }
Set TypingSpeed 90ms
Set Padding 16
Set Framerate 15
Env NO_COLOR ""
Env COLORTERM "truecolor"
Env CLICOLOR "1"

# Start inside the app. Setup and the shell are outside the recording. The
# launch refresh meets the placeholder token and raises an error toast once
# it gives up, which takes up to the API client's 30-second request budget
# when Hardcover is slow; the toast clears eight seconds after that, before
# the camera rolls. Hidden time is not in the recording.
Hide
Type "oku"
Enter
Sleep 40s
Show
Sleep 3s

# The preview pane forecasts each read: pace, time left and the finish day.
# Moving the cursor is enough; opening the detail pane would fetch the
# journal, which the placeholder token refuses.
Type "j"
Sleep 2.5s

# Stats: the goal's pace and the reads finishing next.
Type "4"
Sleep 4s

# Preview oku-light live, then apply it.
Type "T"
Sleep 1s
Type "oku-light"
Sleep 2.5s
Enter
Sleep 2.5s

# The want-to-read shelf, then a published scheme: Nord.
Type "2"
Sleep 2s
Type "T"
Sleep 1s
Type "nord"
Sleep 2s
Enter
Sleep 2s

# Back to Reading and the oku theme through the same picker.
Type "1"
Sleep 1.5s
Type "T"
Type "oku"
Sleep 1.5s
Enter
Sleep 2s

# End on the opening view for a quiet loop; keep the shell out of frame.
Type "k"
Sleep 2s
Hide
Type "q"
Sleep 500ms
