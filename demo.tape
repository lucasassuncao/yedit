# VHS demo tape - https://github.com/charmbracelet/vhs
#
# Regenerate (the vhs container has no Go toolchain, so build the example first):
#
#   bash:
#     CGO_ENABLED=0 GOOS=linux go build -o demo-app ./examples/test
#     docker run --rm -v "$PWD:/vhs" ghcr.io/charmbracelet/vhs demo.tape
#     rm demo-app
#
#   PowerShell:
#     $env:GOOS = "linux"; $env:CGO_ENABLED = "0"
#     go build -o demo-app ./examples/test
#     Remove-Item env:GOOS, env:CGO_ENABLED
#     docker run --rm -v "${PWD}:/vhs" ghcr.io/charmbracelet/vhs demo.tape
#     Remove-Item demo-app
#
# Drives examples/test through a small editing session: toggle the hint panel,
# drill into the server block, apply a preset, save, validate, and quit. The
# example re-creates demo.yaml on first run, so the tape starts from a clean
# seed and removes the file afterwards. Each screen transition is guarded by
# Wait+Screen so the keystrokes cannot desync.

Output docs/demo.gif

Set Shell "bash"
Set FontSize 16
Set Width 1200
Set Height 700
Set TypingSpeed 80ms
Set WaitTimeout 60s

# Fresh seed + launch (hidden: startup is not part of the demo).
Hide
Type "rm -f demo.yaml && ./demo-app"
Enter
Wait+Screen /Blocks/
Show

Sleep 1.5s

# Hint/Example panel for the selected key.
Type "h"
Sleep 2s

# Walk down to the server block (app-name, debug, server, workers).
Down 2
Sleep 1.5s

# Open the server block.
Enter
Wait+Screen /Field/
Sleep 2s

# Browse its fields and apply the "production" preset.
Down
Sleep 1.5s
Type "p"
Wait+Screen /production/
Sleep 1.5s
Down
Sleep 1s
Enter
Sleep 2s

# Commit the block (ctrl+s in the editor returns to the list)...
Ctrl+S
Wait+Screen /not saved yet/
Sleep 1.5s

# ...then save the file (ctrl+s from the list), confirm, and validate.
Ctrl+S
Wait+Screen /Save changes\?/
Sleep 1.5s
Type "y"
Wait+Screen /Saved to/
Sleep 1.5s
Enter
Sleep 1s
Ctrl+L
Wait+Screen /Validation passed/
Sleep 2s
Enter
Sleep 1s

# Quit.
Escape
Sleep 1.5s

# Cleanup the generated seed.
Hide
Type "rm -f demo.yaml"
Enter
