# 01 — Basic editing: a KindPrimitive field (no tree, YAML pane only).
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

# Fresh working copy so the tracked config.yaml is never mutated by recording.
Hide
Type "cp config.yaml session.yaml && ../../demo-app --config session.yaml"
Enter
Wait+Screen /Blocks/
Show

Sleep 1s

# app-name is the first block in the list: a KindPrimitive field, so opening
# it goes straight to the YAML pane - there is no field tree to navigate.
Enter
Wait+Screen /Field/
Sleep 1.5s

# Switch focus to the YAML pane and append a comment to show it is editable
# free-form text, not a fixed form.
Tab
Sleep 500ms
End
Type " # edited"
Sleep 1.5s

# Commit the block (back to the list) without writing to disk.
Ctrl+S
Wait+Screen /not saved yet/
Sleep 1.5s

Escape
Sleep 1s

# Cleanup the working copy.
Hide
Type "rm -f session.yaml"
Enter
