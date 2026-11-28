#!/usr/bin/env bash
# Vercel's ignored build step (apps/web/vercel.json ignoreCommand): exit 0 skips the build, exit 1
# builds. A commit is compared with the last deployed commit of its branch (VERCEL_GIT_PREVIOUS_SHA).
# Without one, production compares with HEAD^ and a preview always builds (its first deployment is
# what the preview workflow binds to the pull request's database). Anything unclear builds.
set -uo pipefail
cd "$(git rev-parse --show-toplevel 2>/dev/null || echo .)" || exit 1
base="${VERCEL_GIT_PREVIOUS_SHA:-}"
if [[ -z "$base" ]]; then
  if [[ "${VERCEL_ENV:-}" == "production" ]]; then base="HEAD^"; else echo "No previous deployment of this branch: build."; exit 1; fi
fi
if ! git cat-file -e "$base^{commit}" 2>/dev/null; then
  git fetch --quiet --depth=100 origin "$base" 2>/dev/null || true
fi
if ! files=$(git diff --name-only "$base" HEAD 2>/dev/null); then
  echo "Cannot compare with $base: build."; exit 1
fi
verdict=$(printf '%s\n' "$files" | node deploy/scripts/web-paths.cjs 2>/dev/null || echo build)
if [[ "$verdict" == "skip" ]]; then
  echo "Nothing the web app uses changed since $base: build skipped."
  exit 0
fi
echo "The web app changed since $base: build."
exit 1
