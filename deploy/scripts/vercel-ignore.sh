#!/usr/bin/env bash
# Vercel's ignored build step (apps/web/vercel.json ignoreCommand): exit 0 skips the build, exit 1
# builds. A commit is compared with the last deployed commit of its branch (VERCEL_GIT_PREVIOUS_SHA).
# Without one (a first deployment) it builds: comparing with HEAD^ would miss the earlier commits of
# a push of several. Renames count under both names (--no-renames). Anything unclear builds.
set -uo pipefail
cd "$(git rev-parse --show-toplevel 2>/dev/null || echo .)" || exit 1
base="${VERCEL_GIT_PREVIOUS_SHA:-}"
if [[ -z "$base" ]]; then echo "No previous deployment of this branch: build."; exit 1; fi
if ! git cat-file -e "$base^{commit}" 2>/dev/null; then
  git fetch --quiet --depth=100 origin "$base" 2>/dev/null || true
fi
if ! files=$(git diff --name-only --no-renames "$base" HEAD 2>/dev/null); then
  echo "Cannot compare with $base: build."; exit 1
fi
verdict=$(printf '%s\n' "$files" | node deploy/scripts/web-paths.cjs 2>/dev/null || echo build)
if [[ "$verdict" == "skip" ]]; then
  echo "Nothing the web app uses changed since $base: build skipped."
  exit 0
fi
echo "The web app changed since $base: build."
exit 1
