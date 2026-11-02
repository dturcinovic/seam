# Records docs/agent.gif with vhs (https://github.com/charmbracelet/vhs):
# task build, then vhs docs/agent.tape from the root of a fresh throwaway
# clone: the note and its answer land in its .git/seam, and S would send
# any note left there from an earlier take too. It needs bash 4, jq and
# the history up to commit 31d2332; the stand-in's errors go to
# .git/agent.log.
#
# docs/agent.sh answers in place of an agent, so the recording needs no
# agent and comes out the same each time. The caret keys count rows of
# that commit's diff in the split view: keep the width.

Output docs/agent.gif

Set Shell bash
Set FontSize 14
Set Width 1800
Set Height 880
Set Padding 12
Set TypingSpeed 60ms

Hide
Type "export PATH=$PWD/bin:$PATH PS1='$ ' XDG_CONFIG_HOME=$(mktemp -d) SEAM_NO_UPDATE_CHECK=1 && set +m"
Enter
Type "docs/agent.sh 'A session that failed to open may still have made the directory. `removeBase` takes it away only when it is empty, so calling it either way is safe.' >/dev/null 2>.git/agent.log &"
Enter
Type "clear"
Enter
Show

Type "seam 31d2332~1 31d2332"
Sleep 500ms
Enter
Sleep 3s

# A question at c.removeBase() in Close.
Tab
Sleep 1.5s
Type "i"
Sleep 400ms
Type "jjjjjjjjjjjjjjj"
Sleep 600ms
Type "c"
Sleep 800ms
Type "Why also without a session?"
Sleep 800ms
Enter
Sleep 1.5s
Escape
Sleep 1s

# Send it to the agent session; its answer turns the note's line to a tick.
Type "S"
Sleep 3s

# The caret on the note opens it, with the answer.
Type "i"
Sleep 400ms
Type "m"
Sleep 4s
Escape
Sleep 500ms

Type "q"
Sleep 500ms
