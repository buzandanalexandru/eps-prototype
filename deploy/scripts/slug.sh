#!/usr/bin/env bash
# Environment slug for a branch (design D0.5); must match apps/web/src/lib/core/url.ts envSlug,
# both checked against deploy/preview-slug-vectors.json.
#   main            -> prod
#   any other name  -> <body>-<6 hex of sha256(branch)>, body = the name with every run of
#                      characters outside ASCII [A-Za-z0-9] turned into "-" (before lowercasing,
#                      so non-ASCII letters never fold into ASCII ones), lowercased, trimmed of
#                      "-", cut to 40 characters; "env" when empty; "br-" in front of a reserved
#                      body (prod, main, staging, eps, kube-*).
# The hash is always added, so no other branch can map to prod or to another branch's slug.
# Usage: deploy/scripts/slug.sh <branch>   ->  prints the slug
set -euo pipefail
branch="${1-}"
if [[ "$branch" == "main" ]]; then echo prod; exit 0; fi
hash=$(printf '%s' "$branch" | sha256sum | cut -c1-6)
body=$(printf '%s' "$branch" | LC_ALL=C sed -E 's/[^A-Za-z0-9]+/-/g' | LC_ALL=C tr '[:upper:]' '[:lower:]' | LC_ALL=C sed -E 's/^-+//; s/-+$//')
body=$(printf '%s' "${body:0:40}" | LC_ALL=C sed -E 's/-+$//')
[[ -n "$body" ]] || body=env
case "$body" in prod | main | staging | eps | kube-*) body="br-$body" ;; esac
slug="$body-$hash"
[[ "$slug" != "prod" ]] || { echo "slug.sh: refusing to map $branch to prod" >&2; exit 3; }
echo "$slug"
