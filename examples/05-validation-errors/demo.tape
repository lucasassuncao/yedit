# 05 — Validation: Ctrl+L runs every configured Validator against the whole
# document. This fixture is missing two required fields (top-level app-name,
# and workers[0].name), both declared via RequiredFromMetadata.
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

Ctrl+L
Wait+Screen /Validation errors/
Sleep 3s

Enter
Sleep 1s

Escape
Sleep 1s

Hide
Type "rm -f session.yaml"
Enter
