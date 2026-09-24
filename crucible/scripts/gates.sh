#!/usr/bin/env bash
# Runs the CI gates from .github/workflows/crucible-go.yml locally, in CI order.
#
#   gates.sh fast   gofmt, go vet, golangci-lint, enginelint, docgate, apiscan
#   gates.sh full   fast + go test -race, covergate, javacycles, prettier, markdownlint
#
# Exit status is non-zero when any gate fails; every gate still runs so one
# pass reports every failure. Output is the failing gates only, plus a summary.
set -u

mode=${1:-full}
case "$mode" in fast | full) ;; *)
	echo "usage: gates.sh [fast|full]" >&2
	exit 64
	;;
esac

repo=$(cd "$(dirname "$0")/../.." && pwd)
cd "$repo/crucible" || exit 1

lint=$(command -v golangci-lint || echo "$(go env GOPATH)/bin/golangci-lint")
failed=()
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

gate() {
	local name=$1
	shift
	if ! "$@" >"$tmp/out" 2>&1; then
		failed+=("$name")
		echo "== FAIL: $name"
		tail -40 "$tmp/out"
	fi
}

gofmt_clean() { test -z "$(gofmt -l .)" || { gofmt -l .; return 1; }; }
golangci() {
	[ -x "$lint" ] || {
		echo "golangci-lint missing: go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.2"
		return 1
	}
	"$lint" run
}

gate gofmt gofmt_clean
gate "go vet" go vet ./...
gate golangci-lint golangci
gate enginelint go run ./tools/enginelint -config internal/engine/enginelint.json
gate docgate go run ./tools/docgate -module . -docs ../docs/crucible
gate "apiscan -check" go run ./tools/apiscan -check
gate "apiscan -check -api" go run ./tools/apiscan -check -api

if [ "$mode" = full ]; then
	gate "go test -race" go test -race -count=1 -coverprofile="$tmp/cover.out" ./...
	[ -s "$tmp/cover.out" ] && gate covergate go run ./tools/covergate -profile "$tmp/cover.out"
	gate javacycles go run ./tools/javacycles -root ../forge-game/src/main/java -prefix forge.game -expect 82
	gate "prettier --check" sh -c 'cd .. && prettier --check . --log-level warn'
	gate markdownlint sh -c 'cd .. && npx --yes markdownlint-cli2 "CLAUDE.md" "docs/crucible/**/*.md"'
fi

if [ ${#failed[@]} -gt 0 ]; then
	echo "gates ($mode): FAILED — ${failed[*]}"
	exit 1
fi
echo "gates ($mode): all green"
