#!/usr/bin/env bash
# Neon branch helper for CI (design D0.6). Never prints a connection string: in GitHub Actions
# the URIs are masked and written to $GITHUB_OUTPUT.
#
#   neon-branch.sh get-or-create <branch-name> [wait-seconds]   (previews: wait for the integration first)
#   neon-branch.sh default                                         (production: the project's default branch)
#   neon-branch.sh delete-if-exists <branch-name>                  (preview cleanup; refuses the default branch)
#
# Needs NEON_API_KEY and NEON_PROJECT_ID. Outputs go to $NEON_OUTPUT_FILE, else $GITHUB_OUTPUT.
set -euo pipefail
: "${NEON_API_KEY:?NEON_API_KEY is not set (GitHub Actions secret NEON_API_KEY)}"
: "${NEON_PROJECT_ID:?NEON_PROJECT_ID is not set}"
API=https://console.neon.tech/api/v2
DB="${NEON_DATABASE:-neondb}"
ROLE="${NEON_ROLE:-neondb_owner}"

neon() {
  local method="$1" path="$2" body="${3-}"
  local args=(-sS --fail-with-body -X "$method" -H "Authorization: Bearer $NEON_API_KEY" -H "Accept: application/json")
  [[ -n "$body" ]] && args+=(-H "Content-Type: application/json" --data "$body")
  curl "${args[@]}" "$API/projects/$NEON_PROJECT_ID$path"
}

find_branch() {
  neon GET "/branches?search=$(jq -rn --arg v "$1" '$v|@uri')" | jq -r --arg n "$1" '[.branches[] | select(.name == $n)][0].id // empty'
}

emit() {
  local id="$1"
  local direct pooled
  direct=$(neon GET "/connection_uri?branch_id=$id&database_name=$DB&role_name=$ROLE&pooled=false" | jq -r .uri)
  pooled=$(neon GET "/connection_uri?branch_id=$id&database_name=$DB&role_name=$ROLE&pooled=true" | jq -r .uri)
  local out="${NEON_OUTPUT_FILE:-${GITHUB_OUTPUT:-}}"
  if [[ -n "${GITHUB_ACTIONS:-}" ]]; then
    echo "::add-mask::$direct"
    echo "::add-mask::$pooled"
  fi
  if [[ -n "$out" ]]; then
    { echo "branch_id=$id"; echo "database_url_unpooled=$direct"; echo "database_url=$pooled"; } >> "$out"
  fi
  echo "Neon branch $id ready (connection strings written to the step outputs, masked)."
}

case "${1-}" in
  get-or-create)
    name="${2:?branch name}"
    wait="${3:-300}"
    id=""
    for ((t = 0; t <= wait; t += 15)); do
      id=$(find_branch "$name")
      [[ -n "$id" ]] && break
      echo "Waiting for the Neon-Vercel integration to create $name ($t s)..."
      sleep 15
    done
    if [[ -z "$id" ]]; then
      echo "Creating Neon branch $name."
      body=$(jq -n --arg n "$name" '{branch: {name: $n}, endpoints: [{type: "read_write"}]}')
      if ! id=$(neon POST "/branches" "$body" | jq -r .branch.id); then
        id=$(find_branch "$name") # created concurrently by the integration: treat as success
      fi
    fi
    [[ -n "$id" ]] || { echo "Could not get or create Neon branch $name" >&2; exit 1; }
    # Wait until the branch's compute accepts connections.
    for _ in $(seq 1 40); do
      state=$(neon GET "/branches/$id" | jq -r .branch.current_state)
      [[ "$state" == "ready" ]] && break
      sleep 5
    done
    emit "$id"
    ;;
  default)
    id=$(neon GET "/branches" | jq -r '[.branches[] | select(.default == true)][0].id')
    emit "$id"
    ;;
  delete-if-exists)
    name="${2:?branch name}"
    [[ "$name" == preview/* ]] || { echo "Refusing to delete $name: only preview/* branches." >&2; exit 2; }
    id=$(find_branch "$name")
    if [[ -z "$id" ]]; then echo "Neon branch $name is already gone."; exit 0; fi
    is_default=$(neon GET "/branches/$id" | jq -r .branch.default)
    [[ "$is_default" == "false" ]] || { echo "Refusing to delete the default branch." >&2; exit 2; }
    neon DELETE "/branches/$id" >/dev/null
    echo "Deleted Neon branch $name ($id)."
    ;;
  *)
    echo "usage: $0 get-or-create <name> [wait] | default | delete-if-exists <name>" >&2
    exit 2
    ;;
esac
