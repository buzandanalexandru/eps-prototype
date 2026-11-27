#!/usr/bin/env bash
# Neon branch helper for CI (design D0.6). Never prints a connection string: URIs go to
# $NEON_OUTPUT_FILE (else $GITHUB_OUTPUT) and are masked in GitHub Actions.
#
#   neon-branch.sh ensure-seed                    the empty seed branch previews are made from
#   neon-branch.sh recreate <eps-preview/...>     delete (if any) and create a preview branch from
#                                                 the seed: no production data, ever
#   neon-branch.sh uri <name>                     owner URIs of a branch (for migrations and roles)
#   neon-branch.sh default                        owner URIs of the default (production) branch
#   neon-branch.sh delete-if-exists <name>        preview cleanup (eps-preview/* or preview/*)
#
# The seed: a schema-only branch of the default branch (Neon copies the structure, never a row),
# with the application schemas then dropped so every preview migrates from version 0. It is a
# root branch, so previews are cheap copy-on-write children of it and use no root allowance.
# Needs NEON_API_KEY and NEON_PROJECT_ID; psql for ensure-seed.
set -euo pipefail
: "${NEON_API_KEY:?NEON_API_KEY is not set (GitHub Actions secret NEON_API_KEY)}"
: "${NEON_PROJECT_ID:?NEON_PROJECT_ID is not set}"
API=https://console.neon.tech/api/v2
DB="${NEON_DATABASE:-neondb}"
ROLE="${NEON_ROLE:-neondb_owner}"
SEED="${NEON_SEED_BRANCH:-eps-preview-seed}"

neon() {
  local method="$1" path="$2" body="${3-}"
  local args=(-sS --fail-with-body -X "$method" -H "Authorization: Bearer $NEON_API_KEY" -H "Accept: application/json")
  [[ -n "$body" ]] && args+=(-H "Content-Type: application/json" --data "$body")
  curl "${args[@]}" "$API/projects/$NEON_PROJECT_ID$path"
}

find_branch() {
  neon GET "/branches?search=$(jq -rn --arg v "$1" '$v|@uri')" | jq -r --arg n "$1" '[.branches[] | select(.name == $n)][0].id // empty'
}

default_id() { neon GET "/branches" | jq -r '[.branches[] | select(.default == true)][0].id'; }

wait_ready() {
  local id="$1" state
  for _ in $(seq 1 60); do
    state=$(neon GET "/branches/$id" | jq -r .branch.current_state)
    [[ "$state" == "ready" ]] && return 0
    sleep 5
  done
  echo "Neon branch $id did not become ready" >&2
  return 1
}

owner_uri() { # <branch id> <pooled true|false>
  neon GET "/connection_uri?branch_id=$1&database_name=$DB&role_name=$ROLE&pooled=$2" | jq -r .uri
}

emit() {
  local id="$1" direct pooled out
  direct=$(owner_uri "$id" false)
  pooled=$(owner_uri "$id" true)
  out="${NEON_OUTPUT_FILE:-${GITHUB_OUTPUT:-}}"
  if [[ -n "${GITHUB_ACTIONS:-}" ]]; then
    echo "::add-mask::$direct"
    echo "::add-mask::$pooled"
  fi
  if [[ -n "$out" ]]; then
    { echo "branch_id=$id"; echo "database_url_unpooled=$direct"; echo "database_url=$pooled"; } >> "$out"
  fi
  echo "Neon branch $id ready (connection strings written to the step outputs, masked)."
}

refuse_default() {
  local id="$1" is_default
  is_default=$(neon GET "/branches/$id" | jq -r .branch.default)
  [[ "$is_default" == "false" ]] || { echo "Refusing to touch the default (production) branch." >&2; exit 2; }
}

delete_id() {
  local id="$1"
  refuse_default "$id"
  neon DELETE "/branches/$id" >/dev/null
  # Wait until the delete operation has finished, so a same-name create does not collide.
  for _ in $(seq 1 30); do
    [[ -z "$(neon GET "/branches/$id" 2>/dev/null | jq -r '.branch.id // empty' 2>/dev/null)" ]] && return 0
    sleep 3
  done
}

case "${1-}" in
  ensure-seed)
    id=$(find_branch "$SEED")
    if [[ -z "$id" ]]; then
      parent=$(default_id)
      echo "Creating the schema-only seed branch $SEED from the default branch (structure only, no rows)."
      body=$(jq -n --arg n "$SEED" --arg p "$parent" \
        '{branch: {name: $n, parent_id: $p, init_source: "schema-only"}, endpoints: [{type: "read_write"}]}')
      id=$(neon POST "/branches" "$body" | jq -r .branch.id)
    fi
    refuse_default "$id"
    wait_ready "$id"
    seed_uri=$(owner_uri "$id" false)
    [[ -n "${GITHUB_ACTIONS:-}" ]] && echo "::add-mask::$seed_uri"
    # Previews migrate from version 0: drop whatever structure the schema-only copy brought.
    n=$(psql "$seed_uri" -tAc "select count(*) from pg_namespace where nspname in ('control', 'sales')")
    if [[ "$n" != "0" ]]; then
      echo "Dropping the application schemas on the seed branch $SEED (it never holds data)."
      psql "$seed_uri" -v ON_ERROR_STOP=1 -qc 'DROP SCHEMA IF EXISTS sales CASCADE; DROP SCHEMA IF EXISTS control CASCADE;'
    fi
    echo "Seed branch $SEED ($id) is empty and ready."
    ;;
  recreate)
    name="${2:?branch name}"
    [[ "$name" == eps-preview/* ]] || { echo "Refusing to recreate $name: only eps-preview/* branches." >&2; exit 2; }
    seed=$(find_branch "$SEED")
    [[ -n "$seed" ]] || { echo "Seed branch $SEED missing: run ensure-seed first." >&2; exit 1; }
    old=$(find_branch "$name")
    if [[ -n "$old" ]]; then
      echo "Deleting the previous $name ($old): every push starts from an empty database."
      delete_id "$old"
    fi
    body=$(jq -n --arg n "$name" --arg p "$seed" '{branch: {name: $n, parent_id: $p}, endpoints: [{type: "read_write"}]}')
    id=$(neon POST "/branches" "$body" | jq -r .branch.id)
    [[ -n "$id" && "$id" != null ]] || { echo "Could not create Neon branch $name" >&2; exit 1; }
    wait_ready "$id"
    emit "$id"
    ;;
  uri)
    name="${2:?branch name}"
    id=$(find_branch "$name")
    [[ -n "$id" ]] || { echo "Neon branch $name not found" >&2; exit 1; }
    emit "$id"
    ;;
  default)
    emit "$(default_id)"
    ;;
  delete-if-exists)
    name="${2:?branch name}"
    [[ "$name" == preview/* || "$name" == eps-preview/* ]] || { echo "Refusing to delete $name: only preview branches." >&2; exit 2; }
    id=$(find_branch "$name")
    if [[ -z "$id" ]]; then echo "Neon branch $name is already gone."; exit 0; fi
    delete_id "$id"
    echo "Deleted Neon branch $name ($id)."
    ;;
  *)
    echo "usage: $0 ensure-seed | recreate <eps-preview/...> | uri <name> | default | delete-if-exists <name>" >&2
    exit 2
    ;;
esac
