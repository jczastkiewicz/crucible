#!/usr/bin/env bash
# ADR-0060's L2 runner: plays every scenario a manifest names on the Java
# engine and writes each post-state into an out dir.
#   crucible/oracle-java/l2-run.sh <manifest> <out dir>
# The manifest comes from `go run ./tools/l2diff emit` (absolute scenario dirs,
# Go's final turn and phase). The script builds the Forge reactor's classes and
# test classes (PlayerControllerForTests lives in forge-gui-desktop's tests) and
# puts them first on the classpath, never the ~/.m2 SNAPSHOT jars, which can
# predate the checked-out source. As in dump-boosters.sh the pom is not used
# (${revision}, ADR-0002). L2_NO_BUILD=1 reuses the last build.
set -euo pipefail
root="$(git rev-parse --show-toplevel)"
manifest="$(cd "$(dirname "$1")" && pwd)/$(basename "$1")"
mkdir -p "$2"
out="$(cd "$2" && pwd)"
# A scenario's result from an earlier run would shadow this one's (l2diff
# reads .unmapped and .error before .state).
find "$out" -maxdepth 1 \( -name '*.state' -o -name '*.unmapped' -o -name '*.error' \) -delete
cd "$root"
work="${L2_WORK:-$root/crucible/oracle-java/target/l2}"
mkdir -p "$work/classes"
if [ -z "${L2_NO_BUILD:-}" ]; then
	mvn -q -B -pl forge-gui-desktop -am test-compile dependency:build-classpath \
		-Dcheckstyle.skip -Dmdep.includeScope=test -Dmdep.outputFile="$work/cp.txt" >&2
fi
cp="$root/forge-gui-desktop/target/test-classes:$root/forge-gui-desktop/target/classes:$(cat "$work/cp.txt")"
if [ -z "${L2_NO_BUILD:-}" ]; then
	javac -nowarn -d "$work/classes" -cp "$cp" crucible/oracle-java/src/main/java/crucible/oracle/ScenarioRunner.java >&2
fi
# Forge finds its res/ tree relative to the working directory, as upstream's
# own tests run it: from forge-gui-desktop.
cd forge-gui-desktop
java -Djava.awt.headless=true -cp "$work/classes:$cp" crucible.oracle.ScenarioRunner "$manifest" "$out"
