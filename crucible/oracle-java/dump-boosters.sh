#!/usr/bin/env bash
# Regenerates crucible/testdata/boosters.golden from Forge's own booster code
# (ADR-0049's gate). Needs `mvn -B -pl forge-core -am install -DskipTests` at the
# repository root once, then:
#   crucible/oracle-java/dump-boosters.sh > crucible/testdata/boosters.golden
# The pom is not used: forge-core's installed pom keeps ${revision} in its parent,
# which Maven cannot resolve outside the reactor, so the classpath comes from
# dependency:build-classpath and javac compiles the one class.
set -euo pipefail
root="$(git rev-parse --show-toplevel)"
cd "$root"
cp_file="$(mktemp)"
classes="$(mktemp -d)"
mvn -q -B -pl forge-core dependency:build-classpath -Dmdep.outputFile="$cp_file" >&2
cp="forge-core/target/classes:$(cat "$cp_file")"
javac -nowarn -d "$classes" -cp "$cp" crucible/oracle-java/src/main/java/crucible/oracle/BoosterDumper.java >&2
java -cp "$classes:$cp" crucible.oracle.BoosterDumper forge-gui/res "${SEEDS:-1,2,3,4,5,6,7,8,9,10,11,12}" all 2>/dev/null
