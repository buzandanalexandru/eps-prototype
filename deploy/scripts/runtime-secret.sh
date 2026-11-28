#!/usr/bin/env bash
# Creates the namespace's runtime secret once and adds any key it lacks (security review on step 3;
# step 4 adds the payment keys): REDIS_PASSWORD (Redis requires it), SNAPSHOT_CACHE_KEY (the core
# signs the copies it keeps in Redis), ORDER_LINK_KEY (resume and order links), PSP_API_KEY (the
# core's key at the simulated providers), PSP_ADMIN_TOKEN (their knobs) and PSP_WEBHOOK_SECRET_A/B
# (their webhook signatures). Random, never printed, and an existing key is never changed, so
# running pods and Redis keep agreeing. Usage: runtime-secret.sh <namespace>
set -euo pipefail
ns="${1:?namespace}"
keys=(REDIS_PASSWORD SNAPSHOT_CACHE_KEY ORDER_LINK_KEY PSP_API_KEY PSP_ADMIN_TOKEN PSP_WEBHOOK_SECRET_A PSP_WEBHOOK_SECRET_B)
if ! kubectl -n "$ns" get secret eps-runtime >/dev/null 2>&1; then
  args=()
  for k in "${keys[@]}"; do args+=(--from-literal="$k=$(openssl rand -hex 32)"); done
  kubectl -n "$ns" create secret generic eps-runtime "${args[@]}" > /dev/null
  echo "eps-runtime created in $ns"
  exit 0
fi
have=$(kubectl -n "$ns" get secret eps-runtime -o json | jq -r '.data | keys[]')
added=0
for k in "${keys[@]}"; do
  if ! grep -qx "$k" <<<"$have"; then
    v=$(openssl rand -hex 32 | tr -d '\n' | base64 -w0)
    kubectl -n "$ns" patch secret eps-runtime --type=json \
      -p "[{\"op\":\"add\",\"path\":\"/data/$k\",\"value\":\"$v\"}]" > /dev/null
    added=$((added + 1))
  fi
done
echo "eps-runtime exists in $ns ($added key(s) added)"
