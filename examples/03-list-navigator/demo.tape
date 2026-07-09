# 03 — [N] navigator: workers is a KindList with child defs, so it gets an
# entry navigator instead of a plain scalar list. Down/Up moves between
# entries; the last row adds a new one.
#
# Prerequisite (run once from the repo root):
#   CGO_ENABLED=0 GOOS=linux go build -o demo-app ./examples/test
#
# Record, from this folder:
#   vhs demo.tape
# See examples/README.md for the full recording workflow (incl. Docker on Windows).

Output demo.gif

Set Shell "bash"
Set FontSize 16
Set Width 1200
Set Height 700
Set TypingSpeed 80ms
Set WaitTimeout 60s

Hide
Type "cp config.yaml session.yaml && ../../demo-app --config session.yaml"
Enter
Wait+Screen /Blocks/
Show

Sleep 1s

# Open the workers block (4th item: app-name, debug, server, workers).
Down 3
Sleep 1s
Enter
Wait+Screen /Field/
Sleep 1.5s

# Move to the next entry ("background").
Down
Sleep 1.5s

# Move to the "+ add new" row and create a third entry.
Down
Sleep 1s
Enter
Sleep 2s

# Back to the list without saving.
Escape
Wait+Screen /Blocks/
Sleep 1s

Escape
Sleep 1s

Hide
Type "rm -f session.yaml"
Enter
