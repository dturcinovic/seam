# Records docs/demo.gif with vhs (https://github.com/charmbracelet/vhs):
# task build, then vhs docs/demo.tape from the repository root. It needs
# gopls on the PATH and the history up to commit 31d2332.
#
# The caret keys count rows and words of that commit's diff in the split
# view, which wants the file list and 120 columns beside it: keep the width.

Output docs/demo.gif

Set Shell bash
Set FontSize 14
Set Width 1800
Set Height 880
Set Padding 12
Set TypingSpeed 60ms

Hide
Type "export PATH=$PWD/bin:$PATH PS1='$ ' XDG_CONFIG_HOME=$(mktemp -d) SEAM_NO_UPDATE_CHECK=1 && clear"
Enter
Show

Type "seam 31d2332~1 31d2332"
Sleep 500ms
Enter
Sleep 4s

# The second file, then a hover on removeSession through the caret.
Tab
Sleep 2s
Type "i"
Sleep 400ms
Type "jjjjjjjjjj"
Sleep 300ms
Type "www"
Sleep 600ms
Type "K"
Sleep 4s
Escape
Sleep 300ms
Escape
Sleep 800ms

# From change to change, then the whole file and back.
Type "n"
Sleep 1s
Type "n"
Sleep 1s
Type "f"
Sleep 2.5s
Type "f"
Sleep 1s

# All files in one stream, on to the next file's header, and back.
Type "s"
Sleep 1.5s
Tab
Sleep 2.5s
Type "s"
Sleep 1s

Type "q"
Sleep 500ms
