#!/usr/bin/env bash
# Shared helpers for the .claude hooks. Sourced, never run.
#
# Every hook resolves the checkout it acts on from the tool call itself (the
# hook input's cwd, the edited file's path, a `git -C` / `cd` in the command),
# never from $CLAUDE_PROJECT_DIR alone. Reason: a subagent in
# .claude/worktrees/<name> is a separate checkout; gating or guarding the main
# checkout instead tests the wrong tree and lets worktree edits through.

# repo_root DIR: the git top level containing DIR, walking up past path
# components that do not exist yet (a Write creating a new directory).
# Falls back to $CLAUDE_PROJECT_DIR.
repo_root() {
	local d=$1
	while [ -n "$d" ] && [ "$d" != / ] && [ ! -d "$d" ]; do d=$(dirname "$d"); done
	git -C "${d:-.}" rev-parse --show-toplevel 2>/dev/null || printf '%s\n' "${CLAUDE_PROJECT_DIR:-.}"
}

# commit_dir CMD CWD: when shell command CMD runs `git commit` (git's
# subcommand, not a word inside `git log --grep=commit` or a message), print
# the directory that commit runs in; print nothing otherwise. Follows a
# preceding `cd DIR`, a subshell's `(cd DIR && ...)`, a no-op wrapper (time,
# sudo, nice, env, ...) in front of `git`, and git's own `-C DIR`.
commit_dir() {
	local cmd=$1 dir=$2 seg rest tok
	while IFS= read -r seg; do
		seg=${seg#"${seg%%[![:space:]]*}"}
		# A subshell `( cd DIR && git commit ... )` splits into segments that
		# still carry the opening paren on the first one; strip it so `cd`/`git`
		# detection below sees the bare command.
		while [[ $seg == \(* ]]; do seg=${seg#\(}; done
		seg=${seg#"${seg%%[![:space:]]*}"}
		# Leading VAR=value assignments do not change the command.
		while [[ $seg =~ ^[A-Za-z_][A-Za-z0-9_]*=[^[:space:]]*[[:space:]]+(.*)$ ]]; do seg=${BASH_REMATCH[1]}; done
		if [[ $seg =~ ^cd[[:space:]]+([^[:space:]]+) ]]; then
			tok=${BASH_REMATCH[1]}
			tok=${tok//\"/}
			tok=${tok//\'/}
			case "$tok" in /*) dir=$tok ;; *) dir=$dir/$tok ;; esac
			continue
		fi
		# Skip a no-op wrapper (time, sudo, nice, env, ...), its flags and any
		# VAR=value assignments it introduces in turn -- what actually execs
		# `git` may be several hops down from the segment's first word.
		while :; do
			if [[ $seg =~ ^[A-Za-z_][A-Za-z0-9_]*=[^[:space:]]*[[:space:]]+(.*)$ ]]; then
				seg=${BASH_REMATCH[1]}
			elif [[ $seg =~ ^-[^[:space:]]*[[:space:]]+(.*)$ ]]; then
				seg=${BASH_REMATCH[1]}
			elif [[ $seg =~ ^(time|sudo|nice|ionice|nohup|env|stdbuf|chrt|command)[[:space:]]+(.*)$ ]]; then
				seg=${BASH_REMATCH[2]}
			else
				break
			fi
		done
		[[ $seg =~ ^git([[:space:]]+(.*))?$ ]] || continue
		rest=${BASH_REMATCH[2]}
		local gdir=$dir
		# Walk git's global options up to the subcommand.
		while [ -n "$rest" ]; do
			tok=${rest%%[[:space:]]*}
			[ "$tok" = "$rest" ] && rest="" || rest=${rest#*[[:space:]]}
			rest=${rest#"${rest%%[![:space:]]*}"}
			case "$tok" in
			-C)
				tok=${rest%%[[:space:]]*}
				[ "$tok" = "$rest" ] && rest="" || rest=${rest#*[[:space:]]}
				tok=${tok//\"/}
				tok=${tok//\'/}
				case "$tok" in /*) gdir=$tok ;; *) gdir=$gdir/$tok ;; esac
				;;
			-c | --git-dir | --work-tree | --namespace)
				rest=${rest#*[[:space:]]}
				;;
			-*) ;;
			commit)
				printf '%s\n' "$gdir"
				return 0
				;;
			*) break ;;
			esac
		done
	done < <(printf '%s\n' "$cmd" | sed -E 's/(&&|\|\||;|\|)/\n/g')
	return 0
}
