#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:1414}"
ADMIN_USERNAME="${ADMIN_USERNAME:-admin}"
ADMIN_PASSWORD="${ADMIN_PASSWORD:-admin}"
USER_EMAIL="${USER_EMAIL:-demo@example.com}"
PLAN_NAME="${PLAN_NAME:-Preview Plan}"

AUTH="${ADMIN_USERNAME}:${ADMIN_PASSWORD}"

curl_json() {
  local method="$1"
  local path="$2"
  local data="${3:-}"
  if [[ -n "$data" ]]; then
    curl -sS -u "$AUTH" -H 'Content-Type: application/json' -X "$method" --data "$data" "$BASE_URL$path"
  else
    curl -sS -u "$AUTH" -X "$method" "$BASE_URL$path"
  fi
}

show() {
  echo
  echo "== $1 =="
  if command -v python3 >/dev/null 2>&1; then
    python3 -m json.tool 2>/dev/null || cat
  else
    cat
  fi
}

echo "Using base URL: $BASE_URL"
echo "Using auth: $ADMIN_USERNAME:$ADMIN_PASSWORD"

echo "1) Health"
curl -sS -u "$AUTH" "$BASE_URL/admin/health" | show "health"

echo "2) List users"
curl_json GET "/admin/users" | show "users"

echo "3) List plans"
curl_json GET "/admin/plans" | show "plans"

echo "4) Capacity"
curl_json GET "/admin/capacity" | show "capacity"

echo "5) Create user"
USER_JSON=$(curl_json POST "/admin/users" "{\"email\":\"$USER_EMAIL\"}")
printf '%s\n' "$USER_JSON" | show "create-user"
USER_ID=$(printf '%s' "$USER_JSON" | python3 -c 'import sys, json; d=json.load(sys.stdin); print(d.get("id") or d.get("user_id") or "")' 2>/dev/null || true)

if [[ -z "$USER_ID" ]]; then
  echo "Could not parse created user ID; skipping assignment test."
  exit 0
fi

echo "6) Create plan"
PLAN_JSON=$(curl_json POST "/admin/plans" "{\"name\":\"$PLAN_NAME\",\"max_total_storage_bytes\":10737418240,\"max_file_size_bytes\":10485760,\"max_files\":100,\"max_files_sent_per_day\":20,\"max_shares_per_day\":10,\"max_files_workspace\":300,\"max_user_workspaces\":5,\"max_total_storage_bytes_workspace\":1073741824,\"max_users_workspace\":10,\"max_workspace_folders\":20,\"max_private_api_keys\":10,\"max_workspace_api_keys\":10}")
printf '%s\n' "$PLAN_JSON" | show "create-plan"
PLAN_ID=$(printf '%s' "$PLAN_JSON" | python3 -c 'import sys, json; d=json.load(sys.stdin); print(d.get("id") or d.get("plan_id") or "")' 2>/dev/null || true)

if [[ -n "$PLAN_ID" ]]; then
  echo "7) Assign plan to user"
  curl_json POST "/admin/users/$USER_ID/plan" "{\"plan_id\":\"$PLAN_ID\"}" | show "assign-plan"
else
  echo "Could not parse created plan ID; skipping assign-plan call."
fi

echo
printf 'Completed.\n'
