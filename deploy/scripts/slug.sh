#!/usr/bin/env bash
# Environment slug for a branch (design D0.5); must match apps/web/src/lib/core/url.ts envSlug.
# Usage: deploy/scripts/slug.sh <branch>   →  prints the slug
set -euo pipefail
branch="${1-}"
if [[ "$branch" == "main" ]]; then echo prod; exit 0; fi
hash=$(printf '%s' "$branch" | sha256sum | cut -c1-6)
clean=$(printf '%s' "$branch" | LC_ALL=C tr '[:upper:]' '[:lower:]' | LC_ALL=C sed -E 's/[^a-z0-9]+/-/g; s/^-+//; s/-+$//')
if [[ -z "$clean" ]]; then echo "env-$hash"; exit 0; fi
if (( ${#clean} <= 40 )); then echo "$clean"; exit 0; fi
cut=$(printf '%s' "${clean:0:40}" | sed -E 's/-+$//')
echo "$cut-$hash"
