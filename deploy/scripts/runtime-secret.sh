#!/usr/bin/env bash
# Creates the namespace's runtime secret once and adds any key it lacks (security review on step 3;
# step 4 adds the payment keys): REDIS_PASSWORD (Redis requires it), SNAPSHOT_CACHE_KEY (the core
# signs the copies it keeps in Redis), ORDER_LINK_KEY (resume and order links), PSP_API_KEY (the
# core's key at the simulated providers), PSP_ADMIN_TOKEN (their knobs) and PSP_WEBHOOK_SECRET_A/B
# (their webhook signatures); step 5 adds the waiting room's pass keys, an Ed25519 pair:
# QUEUE_PASS_PRIVATE_KEY (the 32-byte seed, hex; the gateway only) and QUEUE_PASS_PUBLIC_KEY (hex;
# the core). Random, never printed, and an existing key is never changed, so running pods and
# Redis keep agreeing. Usage: runtime-secret.sh <namespace>
set -euo pipefail
ns="${1:?namespace}"
keys=(REDIS_PASSWORD SNAPSHOT_CACHE_KEY ORDER_LINK_KEY PSP_API_KEY PSP_ADMIN_TOKEN PSP_WEBHOOK_SECRET_A PSP_WEBHOOK_SECRET_B)

# pass_pair prints a JSON patch adding a fresh Ed25519 pair (hex seed and hex public key).
pass_pair() {
  local dir seed pub
  dir=$(mktemp -d)
  openssl genpkey -algorithm ED25519 -out "$dir/k.pem" 2>/dev/null
  # The last 32 bytes of the DER private key are the seed; of the DER public key, the key.
  seed=$(openssl pkey -in "$dir/k.pem" -outform DER 2>/dev/null | tail -c 32 | od -An -tx1 | tr -d ' \n')
  pub=$(openssl pkey -in "$dir/k.pem" -pubout -outform DER 2>/dev/null | tail -c 32 | od -An -tx1 | tr -d ' \n')
  rm -rf "$dir"
  [[ ${#seed} -eq 64 && ${#pub} -eq 64 ]] || { echo "pass key generation failed" >&2; exit 1; }
  printf '[{"op":"add","path":"/data/QUEUE_PASS_PRIVATE_KEY","value":"%s"},{"op":"add","path":"/data/QUEUE_PASS_PUBLIC_KEY","value":"%s"}]' \
    "$(printf %s "$seed" | base64 -w0)" "$(printf %s "$pub" | base64 -w0)"
}

if ! kubectl -n "$ns" get secret eps-runtime >/dev/null 2>&1; then
  args=()
  for k in "${keys[@]}"; do args+=(--from-literal="$k=$(openssl rand -hex 32)"); done
  kubectl -n "$ns" create secret generic eps-runtime "${args[@]}" > /dev/null
  kubectl -n "$ns" patch secret eps-runtime --type=json -p "$(pass_pair)" > /dev/null
  echo "eps-runtime created in $ns"
  exit 0
fi
have=$(kubectl -n "$ns" get secret eps-runtime -o json | jq -r '.data | keys[]')
# The pass key pair is made together: added when neither half exists.
if ! grep -qx QUEUE_PASS_PRIVATE_KEY <<<"$have" && ! grep -qx QUEUE_PASS_PUBLIC_KEY <<<"$have"; then
  pair=$(pass_pair)
  kubectl -n "$ns" patch secret eps-runtime --type=json -p "$pair" > /dev/null
  echo "eps-runtime: pass key pair added"
fi
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
