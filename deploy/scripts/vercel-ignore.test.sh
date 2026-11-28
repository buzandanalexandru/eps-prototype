#!/usr/bin/env bash
# Tests vercel-ignore.sh in a scratch repository: exit 0 = skip, exit 1 = build.
set -euo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
dir=$(mktemp -d); trap 'rm -rf "$dir"' EXIT
cd "$dir"
git init -q . && git config user.email t@example.invalid && git config user.name t
mkdir -p deploy/scripts apps/web services && cp "$here/web-paths.cjs" "$here/vercel-ignore.sh" deploy/scripts/
echo a > apps/web/a.ts; echo s > services/s.go; git add -A; git commit -qm one
first=$(git rev-parse HEAD)
expect() { # expect <want exit> <name> [env...]
  local want=$1 name=$2; shift 2
  set +e; env "$@" bash deploy/scripts/vercel-ignore.sh >/dev/null 2>&1; local got=$?; set -e
  [[ $got == "$want" ]] || { echo "FAIL $name: exit $got, want $want"; exit 1; }
  echo "ok $name"
}
echo s2 > services/s.go; git commit -qam services
expect 0 "services-only change skips (previous deployed sha)" VERCEL_ENV=preview VERCEL_GIT_PREVIOUS_SHA="$first"
expect 1 "no previous deployment builds (preview)" VERCEL_ENV=preview
expect 1 "no previous deployment builds (production: HEAD^ would miss earlier commits)" VERCEL_ENV=production
second=$(git rev-parse HEAD)
echo a2 > apps/web/a.ts; git commit -qam web
echo s3 > services/s.go; git commit -qam services2
expect 1 "a web change since the previous deployment builds" VERCEL_ENV=preview VERCEL_GIT_PREVIOUS_SHA="$first"
expect 1 "an unknown previous sha builds" VERCEL_ENV=preview VERCEL_GIT_PREVIOUS_SHA=0000000000000000000000000000000000000000
mkdir -p api && echo x > api/openapi.yaml && git add -A && git commit -qm api
expect 1 "a contract change builds" VERCEL_ENV=production VERCEL_GIT_PREVIOUS_SHA="$second"
# Review repro (rev6-repros/renametest): moving a file out of apps/web is a web change.
base=$(git rev-parse HEAD)
mkdir -p docs && git mv apps/web/a.ts docs/a.ts && git commit -qm move
expect 1 "a file renamed out of apps/web builds" VERCEL_ENV=preview VERCEL_GIT_PREVIOUS_SHA="$base"
base=$(git rev-parse HEAD)
echo z > strange-new-dir.txt && git add -A && git commit -qm unknown
expect 1 "an unknown top-level path builds" VERCEL_ENV=preview VERCEL_GIT_PREVIOUS_SHA="$base"
base=$(git rev-parse HEAD)
mkdir -p docs && echo d > docs/x.md && git add -A && git commit -qm docs
expect 0 "docs alone skip" VERCEL_ENV=preview VERCEL_GIT_PREVIOUS_SHA="$base"
echo "vercel-ignore.sh: all cases pass"
