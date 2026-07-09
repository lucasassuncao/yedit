# 06 — Hints panel: pressing "h" shows the description, type, default, and
# example for the field under the cursor, sourced from the schema's
# MetadataProvider (Config.EnableHints + Config.Metadata).
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

# Toggle the hint panel for app-name (first item in the list).
Type "h"
Sleep 2.5s

# Move down and watch the hint panel update per field.
Down
Sleep 2s
Down
Sleep 2s

# Hints follow into a block editor too.
Enter
Wait+Screen /Field/
Sleep 1.5s
Type "h"
Sleep 2.5s

Escape
Wait+Screen /Blocks/
Sleep 1s

Escape
Sleep 1s

Hide
Type "rm -f session.yaml"
Enter
