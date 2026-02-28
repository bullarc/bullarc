# VHS tape file for generating bullarc demo GIF.
# Install: brew install charmbracelet/tap/vhs
# Run:     vhs demo.tape
# Or:      make demo-gif

Output demo.gif
Output demo.png

Set Shell "bash"
Set FontSize 14
Set Width 900
Set Height 1000
Set Padding 20
Set TypingSpeed 50ms
Set Theme "Dracula"

Sleep 300ms
Type "bullarc demo"
Sleep 800ms
Enter
Sleep 8s
Screenshot demo.png
Sleep 2s
