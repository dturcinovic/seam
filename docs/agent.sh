#!/usr/bin/env bash
# Stands in for an agent session while docs/agent.tape records: it runs
# seam mcp, waits for the batch that S sends, takes its notes and answers
# each with the text given. It goes only through seam mcp's protocol, so
# the recording shows the way a real answer takes. Needs bash 4 or newer
# (coproc) and jq.
#
#   docs/agent.sh "the answer, in Markdown" &
set -euo pipefail

answer=${1:?usage: agent.sh <answer>}
# Long enough to set a note on camera before S; a stand-in left over from a
# recording that failed should not keep its session.
deadline=$((SECONDS + 120))

fail() {
	echo "agent.sh: $*" >&2
	exit 1
}

coproc MCP { seam mcp; }
# bash unsets these once it reaps the coprocess, which set -u would turn
# into an unrelated error.
mcp_in=${MCP[1]} mcp_out=${MCP[0]} mcp_pid=$MCP_PID

send() { printf '%s\n' "$1" >&"$mcp_in"; }

# next reads the server's messages until one matches the jq filter and
# leaves that match in $res. Neither it nor call may run in $(...): a
# subshell has no coprocess descriptors.
next() {
	local line left
	while left=$((deadline - SECONDS)) && ((left > 0)) &&
		IFS= read -r -t "$left" line <&"$mcp_out"; do
		res=$(jq -c "$1" <<<"$line")
		[[ -n $res ]] && return
	done
	fail "gave up waiting for $1"
}

# call calls a tool and leaves its structured result in $res. A tool that
# fails answers with isError, not with a protocol error.
call() {
	send "$(jq -nc --argjson id "$1" --arg name "$2" --argjson args "$3" \
		'{jsonrpc: "2.0", id: $id, method: "tools/call", params: {name: $name, arguments: $args}}')"
	next "select(.id == $1 and .method == null)"
	jq -e '.error == null and .result.isError != true' <<<"$res" >/dev/null || fail "$2: $res"
	res=$(jq -c '.result.structuredContent' <<<"$res")
}

# seam mcp joins the notes as an agent session once it handles this.
send '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"demo-agent","version":"1"}}}'
next 'select(.id == 1 and .method == null)'
send '{"jsonrpc":"2.0","method":"notifications/initialized"}'

next 'select(.method == "notifications/claude/channel") | .params.meta.batch'
batch=$(jq -r . <<<"$res")

call 2 seam_notes "$(jq -nc --arg b "$batch" '{batch: $b}')"
taken=$res
jq -e '.notes | length > 0' <<<"$taken" >/dev/null || fail "batch $batch gave no notes: $(jq -c .refused <<<"$taken")"
claim=$(jq -r '.claim' <<<"$taken")
id=3
for note in $(jq -r '.notes[].id' <<<"$taken"); do
	call "$id" seam_reply "$(jq -nc --arg n "$note" --arg c "$claim" --arg t "$answer" '{id: $n, claim: $c, text: $t}')"
	id=$((id + 1))
done

# Closing stdin ends seam mcp, which signs the session out.
exec {mcp_in}>&-
wait "$mcp_pid"
