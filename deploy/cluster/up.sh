#!/usr/bin/env bash
# Creates the DOKS cluster for EPS if it does not exist (design D0.5). Idempotent: a second run
# reports the existing cluster and creates nothing. curl + jq against the DigitalOcean API.
#
#   DO_TOKEN=...  deploy/cluster/up.sh        (CI: the DIGITALOCEAN_ACCESS_TOKEN secret)
#   deploy/cluster/up.sh                      (this session: the proxy injects the token)
#
# Region ams3 (EU), because fra1 had no capacity for small nodes on 27 Nov 2026.
set -euo pipefail

NAME="${CLUSTER_NAME:-eps-prototype-ams3}"
REGION="${CLUSTER_REGION:-ams3}"
VERSION="${CLUSTER_VERSION:-1.37.2-do.0}"
POOL_NAME="${POOL_NAME:-pool-main}"
POOL_SIZE="${POOL_SIZE:-s-2vcpu-4gb}"
POOL_MIN="${POOL_MIN:-2}"
POOL_MAX="${POOL_MAX:-4}"
API="${DO_API:-https://api.digitalocean.com/v2}"
TOKEN="${DO_TOKEN:-proxy-injected}"

case "$REGION" in
  ams3|fra1|lon1) ;; # EU regions only (product rule)
  *) echo "Refusing region $REGION: EU regions only." >&2; exit 2 ;;
esac

api() {
  local method="$1" path="$2" body="${3-}"
  local args=(-sS --fail-with-body -X "$method" -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json")
  [[ -n "$body" ]] && args+=(--data "$body")
  curl "${args[@]}" "$API$path"
}

existing=$(api GET "/kubernetes/clusters?per_page=200" | jq -c --arg n "$NAME" '.kubernetes_clusters[] | select(.name == $n)')

if [[ -n "$existing" ]]; then
  echo "Cluster $NAME already exists; creating nothing."
else
  echo "Creating cluster $NAME in $REGION ($VERSION, $POOL_SIZE x $POOL_MIN-$POOL_MAX)..."
  body=$(jq -n --arg name "$NAME" --arg region "$REGION" --arg version "$VERSION" \
    --arg pool "$POOL_NAME" --arg size "$POOL_SIZE" --argjson min "$POOL_MIN" --argjson max "$POOL_MAX" '{
      name: $name, region: $region, version: $version, auto_upgrade: false, surge_upgrade: true,
      ha: false, tags: ["eps-prototype"],
      node_pools: [{ name: $pool, size: $size, count: $min, auto_scale: true, min_nodes: $min, max_nodes: $max }]
    }')
  existing=$(api POST "/kubernetes/clusters" "$body" | jq -c '.kubernetes_cluster')
fi

echo "$existing" | jq -r '"id:      \(.id)
name:    \(.name)
region:  \(.region)
version: \(.version)
status:  \(.status.state)
pools:   \([.node_pools[] | "\(.name) \(.size) \(.min_nodes)-\(.max_nodes) (auto_scale=\(.auto_scale), nodes=\(.nodes | length))"] | join(", "))
api:     \(.endpoint)"'

# The load balancer address is printed by addons.sh once Traefik is installed.
lb=$(api GET "/load_balancers?per_page=200" | jq -r --arg id "$(echo "$existing" | jq -r .id)" \
  '[.load_balancers[] | select(.tag // "" | contains($id)) | .ip] | first // empty' 2>/dev/null || true)
[[ -n "$lb" ]] && echo "lb:      $lb" || echo "lb:      (none yet: run deploy/cluster/addons.sh)"
