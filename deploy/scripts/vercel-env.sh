#!/usr/bin/env bash
# Vercel project environment variables for CI, through the Vercel REST API. Never prints a value.
#   vercel-env.sh upsert <KEY> <value> <plain|encrypted> <targets,comma,separated>
#   vercel-env.sh create-secret-if-missing <KEY> <target>     (48 random bytes, type sensitive)
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
  *) echo "usage: see header" >&2; exit 2 ;;
esac
