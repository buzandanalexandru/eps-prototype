#!/usr/bin/env bash
# Contract drift check (S3): regenerate the TypeScript types and Go stubs from api/openapi.yaml and
# fail, naming the files, if they differ from what is committed.
set -euo pipefail
cd "$(dirname "$0")/.."
generated=(apps/web/src/lib/core/schema.d.ts services/internal/api/api.gen.go)
npm run --silent gen:api -w apps/web >/dev/null
(cd services && make --silent gen)
if ! git diff --quiet -- "${generated[@]}"; then
  echo "Contract drift: these generated files differ from api/openapi.yaml:" >&2
  git diff --name-only -- "${generated[@]}" | sed 's/^/  - /' >&2
  echo "Run 'npm run gen:api' at the repository root and commit the result." >&2
  exit 1
fi
echo "Contract and generated files are in sync."
