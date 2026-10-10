#!/bin/sh
# Builds the classic module images. Run from the repository root with docker
# or podman on PATH:  sh modules/classic/build.sh [world movement combat time]
set -eu
engine=$(command -v docker || command -v podman)
for m in ${*:-world movement combat time}; do
  "$engine" build -q -f modules/classic/Dockerfile --build-arg MODULE="$m" -t "localhost/agent-wow-classic-$m:dev" .
done
