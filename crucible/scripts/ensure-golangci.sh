#!/usr/bin/env bash
# Makes sure a golangci-lint that can lint this module is installed, and
# prints its path. No-op (well under a second) when one already is.
#
# golangci-lint refuses a module whose `go` line is newer than the Go it was
# built with: "the Go language version (go1.25) used to build golangci-lint
# is lower than the targeted Go version (1.27.0)". A plain
#   go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.2
# does not fix that: go install honours golangci-lint's own go.mod and
# switches to go1.26.x. Building with the module's pinned toolchain binary,
# GOTOOLCHAIN=local, does. The version is CI's (.github/workflows/
# crucible-go.yml).
#
#   ensure-golangci.sh          install if needed, print the usable binary
#   ensure-golangci.sh -check   print it, or exit 1 without installing
set -euo pipefail
version=v2.13.2
cd "$(dirname "$0")/.."

want=$(sed -nE 's/^go ([0-9]+\.[0-9]+).*/\1/p' go.mod)

# usable reports whether binary $1 was built with a Go >= the module's.
usable() {
	[ -x "$1" ] || return 1
	local built
	built=$("$1" version 2>/dev/null | sed -nE 's/.*built with go([0-9]+\.[0-9]+).*/\1/p')
	[ -n "$built" ] || return 1
	[ "$(printf '%s\n%s\n' "$want" "$built" | sort -V | head -1)" = "$want" ]
}

gobin=$(go env GOPATH)/bin/golangci-lint
for c in "$gobin" "$(command -v golangci-lint || true)"; do
	if [ -n "$c" ] && usable "$c"; then
		echo "$c"
		exit 0
	fi
done
[ "${1:-}" = "-check" ] && exit 1

# `go env GOROOT` resolves the toolchain go.mod pins (downloading it once).
GOTOOLCHAIN=local "$(go env GOROOT)/bin/go" install "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$version" >&2
usable "$gobin" || {
	echo "ensure-golangci: $gobin still not built with go$want" >&2
	exit 1
}
echo "$gobin"
