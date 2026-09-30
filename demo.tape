# VHS demo tape - https://github.com/charmbracelet/vhs
#
# Written for Windows: with vhs, ttyd and ffmpeg on the PATH, record from the
# repo root (Output resolves against the working directory):
#
#   vhs demo.tape
#
# The tape builds demo.exe off camera and deletes it when done.
#
# A tour of cmd/demo: the block list and its hints, a block's field tree,
# a preset applied from its preview, the [N] list navigator, two-level undo,
# validation, the save confirmation, and the key help. The demo re-creates
# demo.yaml on first run, so the tape starts from a clean seed and removes the
# file afterwards. Screen changes are guarded by Wait+Screen so the keystrokes
# cannot run ahead of the editor.

Output docs/demo.gif

Require ttyd
Require ffmpeg

Set Shell "powershell"
Set FontSize 16
Set Width 1200
Set Height 700
Set Padding 20
Set Theme "Catppuccin Mocha"
Set TypingSpeed 80ms
Set WaitTimeout 30s
Set Framerate 20

# Build, fresh seed + launch (hidden: startup is not part of the demo).
Hide
# A NO_COLOR inherited from the parent shell would record in monochrome.
Type "Remove-Item Env:NO_COLOR -ErrorAction SilentlyContinue"
Enter
Wait
Type "go build -o demo.exe ./cmd/demo"
Enter
Wait
Type "Remove-Item demo.yaml -ErrorAction SilentlyContinue; clear; .\demo.exe"
Enter
Wait+Screen /Blocks/
Show

# The block list: the hint panel follows the cursor.
Sleep 2s
Down
Sleep 1.2s
Down
Sleep 1.5s

# Open the server block: its fields as a tree, with a nested struct.
Enter
Wait+Screen /Fields/
Sleep 1.5s
Down 2
Sleep 0.8s
Right
Sleep 1.5s

# Apply a preset from its preview.
Type "p"
Wait+Screen /Available Presets/
Sleep 1.5s
Down
Sleep 2s
Enter
Wait+Screen /Uncommitted changes/
Sleep 2s

# Commit the block back into the document.
Ctrl+S
Wait+Screen /not saved yet/
Sleep 2s

# A list opens in the [N] navigator: one row per entry.
Down
Enter
Wait+Screen /add new/
Sleep 1.5s
Right
Sleep 2s
Escape
Wait+Screen /Blocks/
Sleep 1s

# Undo the whole server commit from the list, then redo it.
Ctrl+U
Sleep 2s
Ctrl+Y
Sleep 2s

# Validate, then save through the confirmation.
Ctrl+L
Wait+Screen /Validation passed/
Sleep 2.5s
Enter
Sleep 1s
Ctrl+S
Wait+Screen /Save changes/
Sleep 1.5s
Right
Sleep 1s
Left
Sleep 1s
Enter
Wait+Screen /Saved to/
Sleep 2s
Enter
Sleep 1s

# Every key, one keystroke away.
Type "?"
Sleep 3s
Escape
Sleep 1s

# Quit.
Type "q"
Sleep 1s

# Cleanup the generated seed and the binary.
Hide
Type "Remove-Item demo.yaml, demo.exe"
Enter
Wait
