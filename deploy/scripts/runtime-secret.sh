#!/usr/bin/env bash
# Creates the namespace's runtime secret once (security review on step 3): REDIS_PASSWORD (Redis
# requires it) and SNAPSHOT_CACHE_KEY (the core signs the copies it keeps in Redis). Random, never
# printed, kept across deploys so running pods and Redis agree. Usage: runtime-secret.sh <namespace>
set -euo pipefail
ns="${1:?namespace}"
if kubectl -n "$ns" get secret eps-runtime >/dev/null 2>&1; then
  echo "eps-runtime exists in $ns"
  exit 0
fi
kubectl -n "$ns" create secret generic eps-runtime \
  --from-literal=REDIS_PASSWORD="$(openssl rand -hex 32)" \
  --from-literal=SNAPSHOT_CACHE_KEY="$(openssl rand -hex 32)" > /dev/null
echo "eps-runtime created in $ns"
