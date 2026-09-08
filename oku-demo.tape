Output oku-demo.gif
Output oku-demo.mp4
Set FontSize 18
Set Width 1400
Set Height 1000
Set Theme "Catppuccin Mocha"
Set TypingSpeed 65ms
Set Padding 20
Set Framerate 15

# The palettes, one swatch row each, on their own surfaces.
Type "oku config theme --preview"
Enter
Sleep 4s

# Naming one writes it to the config; the dashboard draws with it from the next start.
Type "oku config theme catppuccin-mocha"
Enter
Sleep 2s

Type "oku"
Enter
Sleep 3s

# Browse the sample reading library. The detail pane follows the cursor.
Type "j"
Sleep 1s
Type "j"
Sleep 1s
Type "k"
Sleep 1s

# Enter moves the keyboard into the detail pane; Escape brings it back.
Enter
Sleep 2s
Escape
Sleep 1s

# Want-to-read shelf: tab 2.
Type "l"
Sleep 2s
Type "j"
Sleep 1s

# Past the Search tab (3) to the statistics (4); no live search is made.
Type "ll"
Sleep 4s

# The Timer tab (5): open its local book picker, then cancel.
Type "l"
Sleep 2s
Type "t"
Sleep 2s
Type "j"
Sleep 1s
Escape
Sleep 1s

# The help modal names the palette in its title.
Type "?"
Sleep 2.5s
Escape
Sleep 1s

# Back to the reading library, then out.
Type "hhhh"
Sleep 2s
Type "q"
Sleep 1s

# The same dashboard in Nord.
Type "oku config theme nord"
Enter
Sleep 1.5s
Type "oku"
Enter
Sleep 3s
Type "l"
Sleep 2s
Type "ll"
Sleep 4s
Type "q"
Sleep 1s
