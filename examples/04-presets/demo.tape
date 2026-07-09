# 04 — Presets: pressing "p" inside a block with BlockPresets configured
# opens a picker of ready-made YAML snippets (here, "minimal" and
# "production" server configs) that replace the block's content.
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

# Open the server block.
Down 2
Sleep 1s
Enter
Wait+Screen /Field/
Sleep 1.5s

# "p" opens the preset picker regardless of cursor position - it acts on the
# whole block, not the focused field.
Type "p"
Wait+Screen /production/
Sleep 1.5s

# Presets are listed alphabetically: minimal, then production.
Down
Sleep 1s
Enter
Sleep 2s

# Commit the block (back to the list) without writing to disk.
Ctrl+S
Wait+Screen /not saved yet/
Sleep 1.5s

Escape
Sleep 1s

Hide
Type "rm -f session.yaml"
Enter
