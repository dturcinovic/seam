# Records docs/notes.gif with vhs (https://github.com/charmbracelet/vhs):
# task build, then vhs docs/notes.tape from the root of a fresh throwaway
# clone: the notes it sets land in its .git/seam, and notes left there
# from an earlier take would show too. It needs the history up to commit
# 31d2332.
#
# The caret keys count rows of that commit's diff in the split view, which
# wants the file list and 120 columns beside it: keep the width.

Output docs/notes.gif

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
Sleep 3s

# A question at c.removeBase() in Close, with a look at the other kind.
Tab
Sleep 1.5s
Type "i"
Sleep 400ms
Type "jjjjjjjjjjjjjjj"
Sleep 600ms
Type "c"
Sleep 1.2s
Tab
Sleep 1s
Tab
Sleep 600ms
Type "Why also without a session?"
Sleep 800ms
Enter
Sleep 2s

# A finding at the same call in Clean.
Type "jjjjjjjjjjjj"
Sleep 600ms
Type "c"
Sleep 800ms
Tab
Sleep 600ms
Type "Same call as in Close: worth one test for both."
Sleep 800ms
Enter
Sleep 2s
Escape
Sleep 1s

# Every note of the worktree, and on to the finding.
Type "N"
Sleep 2.5s
Type "j"
Sleep 800ms
Enter
Sleep 2.5s

Type "q"
Sleep 500ms
