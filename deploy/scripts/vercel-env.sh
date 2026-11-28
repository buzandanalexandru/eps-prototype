#!/usr/bin/env bash
# Vercel project environment variables and deployments for CI, through the Vercel REST API. Never
# prints a value; secret values are read from the environment variable named, never from argv.
#   vercel-env.sh upsert <KEY> <value> <plain|encrypted> <targets,comma,separated>
#   vercel-env.sh create-secret-if-missing <KEY> <target>     (48 random bytes, type sensitive)
#   vercel-env.sh upsert-branch <KEY> <VALUE_VAR> <git-branch> (preview only, scoped to one branch)
#   vercel-env.sh delete-branch <KEY> <git-branch>
#   vercel-env.sh get-branch <KEY> <git-branch> <FILE>        writes the branch-scoped value to FILE
#                                                             (empty when unset); never prints it
#   vercel-env.sh exists <KEY> <target>                       prints true or false
#   vercel-env.sh upsert-secret <KEY> <VALUE_VAR> <target>    (production: type sensitive)
#   vercel-env.sh describe <deployment-host>                  prints url= and origins= lines
#   vercel-env.sh redeploy <deployment-host> [production]     rebuilds it with the current env;
#                                                             prints url= and origins= lines
set -euo pipefail
: "${VERCEL_TOKEN:?VERCEL_TOKEN is not set (GitHub Actions secret VERCEL_TOKEN)}"
PROJECT="${VERCEL_PROJECT_ID:-prj_Wxv6KSuf1LnYTusYGNwtuHdkMXvD}"
TEAM="${VERCEL_TEAM_ID:-team_pR8jlYM98Wtxqefs0u6pHuXP}"
API="https://api.vercel.com"

v() {
  local method="$1" path="$2" body="${3-}"
  local args=(-sS --fail-with-body -X "$method" -H "Authorization: Bearer $VERCEL_TOKEN")
  [[ -n "$body" ]] && args+=(-H "Content-Type: application/json" --data "$body")
  curl "${args[@]}" "$API$path"
}

case "${1-}" in
  upsert)
    key="$2" value="$3" type="$4" targets="$5"
    body=$(jq -n --arg k "$key" --arg v "$value" --arg t "$type" --arg targets "$targets" \
      '{key: $k, value: $v, type: $t, target: ($targets | split(","))}')
    v POST "/v10/projects/$PROJECT/env?teamId=$TEAM&upsert=true" "$body" > /dev/null
    echo "Set $key on $targets."
    ;;
  create-secret-if-missing)
    key="$2" target="$3"
    exists=$(v GET "/v10/projects/$PROJECT/env?teamId=$TEAM" |
      jq --arg k "$key" --arg t "$target" '[.envs[] | select(.key == $k and (.target | index($t)))] | length')
    if [[ "$exists" != "0" ]]; then echo "$key already exists for $target; kept."; exit 0; fi
    value=$(openssl rand -base64 48 | tr -d '\n')
    echo "::add-mask::$value"
    type=sensitive
    [[ "$target" == "development" ]] && type=encrypted # sensitive is not allowed for development
    body=$(jq -n --arg k "$key" --arg v "$value" --arg t "$target" --arg ty "$type" '{key: $k, value: $v, type: $ty, target: [$t]}')
    v POST "/v10/projects/$PROJECT/env?teamId=$TEAM" "$body" > /dev/null
    echo "Created $key for $target."
    ;;
  upsert-branch)
    key="$2" var="$3" branch="$4"
    value="${!var:?$var is empty}"
    body=$(jq -n --arg k "$key" --arg v "$value" --arg b "$branch" \
      '{key: $k, value: $v, type: "encrypted", target: ["preview"], gitBranch: $b}')
    v POST "/v10/projects/$PROJECT/env?teamId=$TEAM&upsert=true" "$body" > /dev/null
    echo "Set $key for preview branch $branch."
    ;;
  delete-branch)
    key="$2" branch="$3"
    ids=$(v GET "/v10/projects/$PROJECT/env?teamId=$TEAM&gitBranch=$(jq -rn --arg b "$branch" '$b|@uri')" |
      jq -r --arg k "$key" --arg b "$branch" '.envs[] | select(.key == $k and .gitBranch == $b) | .id')
    for id in $ids; do v DELETE "/v9/projects/$PROJECT/env/$id?teamId=$TEAM" > /dev/null; done
    echo "Removed $key for preview branch $branch."
    ;;
  get-branch)
    key="$2" branch="$3" file="$4"
    id=$(v GET "/v10/projects/$PROJECT/env?teamId=$TEAM&gitBranch=$(jq -rn --arg b "$branch" '$b|@uri')" |
      jq -r --arg k "$key" --arg b "$branch" '[.envs[] | select(.key == $k and .gitBranch == $b)][0].id // empty')
    : > "$file"
    if [[ -n "$id" ]]; then v GET "/v9/projects/$PROJECT/env/$id?teamId=$TEAM&decrypt=true" | jq -r '.value // empty' > "$file"; fi
    ;;
  exists)
    key="$2" target="$3"
    n=$(v GET "/v10/projects/$PROJECT/env?teamId=$TEAM" |
      jq --arg k "$key" --arg t "$target" '[.envs[] | select(.key == $k and (.target | index($t)) and (.gitBranch == null))] | length')
    [[ "$n" != "0" ]] && echo true || echo false
    ;;
  upsert-secret)
    key="$2" var="$3" target="$4"
    value="${!var:?$var is empty}"
    body=$(jq -n --arg k "$key" --arg v "$value" --arg t "$target" '{key: $k, value: $v, type: "sensitive", target: [$t]}')
    v POST "/v10/projects/$PROJECT/env?teamId=$TEAM&upsert=true" "$body" > /dev/null
    echo "Set $key for $target."
    ;;
  describe)
    host="${2:?deployment host}"
    host="${host#https://}"; host="${host%%/*}"
    d=$(v GET "/v13/deployments/$host?teamId=$TEAM")
    echo "url=https://$(jq -r .url <<<"$d")"
    echo "origins=$(jq -r '[("https://" + .url)] + [(.alias // [])[] | "https://" + .] | unique | join(",")' <<<"$d")"
    ;;
  redeploy)
    host="${2:?deployment host}" target="${3-}"
    host="${host#https://}"; host="${host%%/*}"
    src=$(v GET "/v13/deployments/$host?teamId=$TEAM")
    id=$(jq -r .id <<<"$src"); name=$(jq -r .name <<<"$src")
    body=$(jq -n --arg id "$id" --arg n "$name" --arg t "$target" \
      '{deploymentId: $id, name: $n} + (if $t == "" then {} else {target: $t} end)')
    new=$(v POST "/v13/deployments?teamId=$TEAM&forceNew=1" "$body" | jq -r .id)
    echo "Redeploying $host as $new so it picks up the new variables..." >&2
    for _ in $(seq 1 120); do
      d=$(v GET "/v13/deployments/$new?teamId=$TEAM")
      state=$(jq -r .readyState <<<"$d")
      case "$state" in
        READY)
          echo "url=https://$(jq -r .url <<<"$d")"
          echo "origins=$(jq -r '[("https://" + .url)] + [(.alias // [])[] | "https://" + .] | unique | join(",")' <<<"$d")"
          exit 0 ;;
        ERROR | CANCELED) echo "Vercel redeploy $new ended $state" >&2; exit 1 ;;
      esac
      sleep 10
    done
    echo "Vercel redeploy $new not ready after 20 minutes" >&2; exit 1
    ;;
  *) echo "usage: see header" >&2; exit 2 ;;
esac
