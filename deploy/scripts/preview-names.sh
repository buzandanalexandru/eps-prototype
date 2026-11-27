#!/usr/bin/env bash
# Names of a pull request's environment (design D0.5, D0.6), one per line as key=value, for
# preview.yml and preview-cleanup.yml. Every name is derived from the branch's slug, which always
# carries the branch hash, so two pull requests never share a namespace, release, database branch
# or Vercel variable scope. Refuses (exit 3) anything that would touch production.
#   deploy/scripts/preview-names.sh <branch>
set -euo pipefail
branch="${1:?branch name}"
[[ "$branch" != "main" ]] || { echo "preview-names.sh: main is production, not a preview" >&2; exit 3; }
slug=$("$(dirname "$0")/slug.sh" "$branch")
ns="eps-$slug"
if [[ "$slug" == "prod" || "$ns" == "eps-prod" ]]; then
  echo "preview-names.sh: refusing to name a preview after production ($branch)" >&2
  exit 3
fi
echo "slug=$slug"
echo "namespace=$ns"
echo "release=$ns"
# CI's own branch: a child of the empty seed branch, never of production.
echo "neon_branch=eps-preview/$slug"
# The Neon-Vercel integration's branch for the same git branch (copied from production with its
# data). Never used; deleted on cleanup.
echo "integration_branch=preview/$branch"
echo "vercel_git_branch=$branch"
