#!/usr/bin/env bash
# Fails with a clear message naming every missing secret (CI must never skip silently).
# Usage: require-secrets.sh NAME...   (each NAME is read from the environment)
set -euo pipefail
missing=()
for name in "$@"; do
  [[ -n "${!name:-}" ]] || missing+=("$name")
done
if ((${#missing[@]})); then
  for m in "${missing[@]}"; do
    echo "::error title=Missing secret::The GitHub Actions secret $m is not set. Add it in Settings > Secrets and variables > Actions."
  done
  exit 1
fi
echo "Secrets present: $*"
