#!/usr/bin/env bash
# R21a: plays one Java AI-vs-AI game and writes its replay log (ADR-0059,
# ADR-0062) as JSON Lines.
#   crucible/oracle-java/record-game.sh <deck A .dck> <deck B .dck> <seed> <out .jsonl>
# Builds and runs like l2-run.sh: the Forge reactor's classes first on the
# classpath, never ~/.m2 SNAPSHOT jars; L2_NO_BUILD=1 reuses the last build.
# RecordingControllerBase.java is generated: after an upstream sync that
# touches PlayerController, regenerate it with REGEN=1.
set -euo pipefail
root="$(git rev-parse --show-toplevel)"
abs() { echo "$(cd "$(dirname "$1")" && pwd)/$(basename "$1")"; }
deck_a="$(abs "$1")"
deck_b="$(abs "$2")"
seed="$3"
mkdir -p "$(dirname "$4")"
out="$(abs "$4")"
cd "$root"
work="${L2_WORK:-$root/crucible/oracle-java/target/l2}"
mkdir -p "$work/rec"
if [ -z "${L2_NO_BUILD:-}" ]; then
	mvn -q -B -pl forge-gui-desktop -am test-compile dependency:build-classpath \
		-Dcheckstyle.skip -Dmdep.includeScope=test -Dmdep.outputFile="$work/cp.txt" >&2
fi
cp="$root/forge-gui-desktop/target/test-classes:$root/forge-gui-desktop/target/classes:$(cat "$work/cp.txt")"
src=crucible/oracle-java/src/main/java/crucible/oracle
if [ -n "${REGEN:-}" ]; then
	javac -nowarn -d "$work/rec" -cp "$cp" "$src/RecorderGen.java" >&2
	java -cp "$work/rec:$cp" crucible.oracle.RecorderGen "$src/RecordingControllerBase.java"
fi
if [ -z "${L2_NO_BUILD:-}" ] || [ -n "${REGEN:-}" ]; then
	javac -nowarn -d "$work/rec" -cp "$cp" "$src"/RecorderGen.java "$src"/Recording*.java "$src"/Replay*.java \
		"$src"/GameRecorder.java >&2
fi
commit="$(git rev-parse --short=11 HEAD)"
cd forge-gui-desktop
java -Djava.awt.headless=true -cp "$work/rec:$cp" crucible.oracle.GameRecorder "$deck_a" "$deck_b" "$seed" "$commit" "$out"
