# 02 — Nested drill-in: server.pool is a struct nested inside another struct.
# Enter/right on it opens a second, stacked block editor scoped to just that
# field; Esc pops back to the parent, keeping whatever was edited.
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

# Open the server block (3rd item: app-name, debug, server).
Down 2
Sleep 1s
Enter
Wait+Screen /Field/
Sleep 1.5s

# Move down to the pool field (host, port, pool) and drill into it.
Down 2
Sleep 1s
Right
Wait+Screen /Field/
Sleep 2s

# Browse pool's own fields (min-size, max-size).
Down
Sleep 1.5s

# Esc pops back to the parent (server), keeping the pool subtree as-is.
Escape
Wait+Screen /Field/
Sleep 1.5s

# Esc again leaves the block editor entirely, back to the list.
Escape
Wait+Screen /Blocks/
Sleep 1s

Escape
Sleep 1s

Hide
Type "rm -f session.yaml"
Enter
