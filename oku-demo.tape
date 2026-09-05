Output oku-demo.mp4
Set FontSize 18
Set Width 1400
Set Height 1000 
Set Theme "Catppuccin Mocha"
Set TypingSpeed 55ms
Set Padding 20

Sleep 1s
Type "OKU_TUI_DEMO_DATA=1 ./oku-dev tui"
Sleep 700ms
Enter
Sleep 5s

# 1) Intro
Type "h"
Sleep 2000ms

# 2) Reading
Type "l"
Sleep 2400ms
Type "j"
Sleep 1700ms
Type "k"
Sleep 2100ms

# 3) Oku
Type "l"
Sleep 2400ms
Type "j"
Sleep 1700ms
Type "j"
Sleep 1700ms
Type "k"
Sleep 2100ms

# 4) Search Titles
Type "l"
Sleep 2400ms
Type "i"
Sleep 700ms
Type "dune"
Sleep 900ms
Enter
Sleep 4200ms

# Move in results, then increase density (verbose view)
Type "j"
Sleep 1800ms
Type "j"
Sleep 1800ms
Type "k"
Sleep 1800ms
Type "z"
Sleep 3200ms

# Go back to input, then return to results without re-searching
Type "h"
Sleep 1800ms
Type "l"
Sleep 2600ms

# 5) Stats
Type "l"
Sleep 2800ms

# 6) Streak
Type "l"
Sleep 2800ms

# 7) Timer (manual book picker)
Type "l"
Sleep 2400ms
Type "t"
Sleep 2400ms
Type "j"
Sleep 1700ms
Type "k"
Sleep 1700ms
Escape
Sleep 2200ms

Type "q"
Sleep 2s
