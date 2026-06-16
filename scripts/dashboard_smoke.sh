#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

if ! command -v npx >/dev/null 2>&1; then
  echo "npx is required for Playwright smoke tests." >&2
  exit 1
fi

CODEX_HOME="${CODEX_HOME:-$HOME/.codex}"
PWCLI="${PWCLI:-$CODEX_HOME/skills/playwright/scripts/playwright_cli.sh}"
if [[ ! -f "$PWCLI" ]]; then
  echo "Playwright wrapper not found at $PWCLI" >&2
  exit 1
fi

APP_HOST="${APP_HOST:-127.0.0.1}"
APP_PORT="${APP_PORT:-18080}"
BASE_URL="http://${APP_HOST}:${APP_PORT}"

ADMIN_EMAIL="${ADMIN_EMAIL:-admin@example.com}"
ADMIN_PASSWORD="${ADMIN_PASSWORD:-Pass123!}"

INFERENCE_PROVIDER="${INFERENCE_PROVIDER:-privatemode}"
PRIVATEMODE_BASE_URL="${PRIVATEMODE_BASE_URL:-https://example.com/v1}"
PRIVATEMODE_API_KEY="${PRIVATEMODE_API_KEY:-test}"

SERVER_PID=""
cleanup() {
  bash "$PWCLI" close-all >/dev/null 2>&1 || true
  if [[ -n "$SERVER_PID" ]]; then
    kill "$SERVER_PID" >/dev/null 2>&1 || true
    wait "$SERVER_PID" >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

run_pw() {
  local cmd="$*"
  local output
  output="$(bash "$PWCLI" "$@" 2>&1)" || {
    echo "Playwright command failed: $cmd" >&2
    echo "$output" >&2
    return 1
  }
  if [[ "$output" == *"### Error"* ]]; then
    echo "Playwright command reported error: $cmd" >&2
    echo "$output" >&2
    return 1
  fi
  echo "$output" >/dev/null
}

js_escape() {
  local s="$1"
  s="${s//\\/\\\\}"
  s="${s//\'/\\\'}"
  s="${s//$'\n'/ }"
  printf "%s" "$s"
}

make build >/dev/null

APP_HOST="$APP_HOST" \
APP_PORT="$APP_PORT" \
INFERENCE_PROVIDER="$INFERENCE_PROVIDER" \
PRIVATEMODE_BASE_URL="$PRIVATEMODE_BASE_URL" \
PRIVATEMODE_API_KEY="$PRIVATEMODE_API_KEY" \
ADMIN_EMAIL="$ADMIN_EMAIL" \
ADMIN_PASSWORD="$ADMIN_PASSWORD" \
./enclava >/tmp/enclava-dashboard-smoke.log 2>&1 &
SERVER_PID="$!"

for _ in $(seq 1 80); do
  if curl -fsS "${BASE_URL}/livez" >/dev/null 2>/dev/null; then
    break
  fi
  sleep 0.25
done
if ! curl -fsS "${BASE_URL}/livez" >/dev/null 2>/dev/null; then
  echo "Service did not become ready at ${BASE_URL}/livez" >&2
  tail -n 80 /tmp/enclava-dashboard-smoke.log >&2 || true
  exit 1
fi

bash "$PWCLI" close-all >/dev/null 2>&1 || true
run_pw open "${BASE_URL}/dashboard"

EMAIL_JS="$(js_escape "$ADMIN_EMAIL")"
PASSWORD_JS="$(js_escape "$ADMIN_PASSWORD")"
BASE_URL_JS="$(js_escape "$BASE_URL")"

run_pw run-code "async (page) => { await page.waitForURL('**/dashboard/login'); }"
run_pw run-code "async (page) => { await page.getByRole('textbox', { name: 'Email' }).fill('${EMAIL_JS}'); }"
run_pw run-code "async (page) => { await page.getByRole('textbox', { name: 'Password' }).fill('${PASSWORD_JS}'); }"
run_pw run-code "async (page) => { await page.getByRole('button', { name: 'Sign in' }).click(); }"
run_pw run-code "async (page) => { await page.waitForURL('**/dashboard'); }"
run_pw run-code "async (page) => { await page.getByRole('heading', { name: 'Dashboard' }).waitFor(); }"
run_pw run-code "async (page) => { await page.getByText('${EMAIL_JS}', { exact: false }).first().waitFor(); }"
run_pw run-code "async (page) => { await page.getByText('Active API Keys', { exact: false }).first().waitFor(); }"
run_pw run-code "async (page) => { await page.getByText('Budget Usage', { exact: false }).first().waitFor(); }"
run_pw run-code "async (page) => { await page.getByText('API Endpoint', { exact: false }).first().waitFor(); }"
run_pw run-code "async (page) => { const res = await page.request.get('${BASE_URL_JS}/dashboard/partials/stats'); if (res.status() !== 200) { throw new Error('stats status ' + res.status()); } }"
run_pw run-code "async (page) => { const res = await page.request.get('${BASE_URL_JS}/dashboard/partials/usage'); if (res.status() !== 200) { throw new Error('usage status ' + res.status()); } }"
run_pw run-code "async (page) => { const res = await page.request.get('${BASE_URL_JS}/dashboard/partials/keys'); if (res.status() !== 200) { throw new Error('keys status ' + res.status()); } }"
run_pw run-code "async (page) => { await page.getByRole('button', { name: 'Sign out' }).click(); }"
run_pw run-code "async (page) => { await page.waitForURL('**/dashboard/login'); }"

echo "Dashboard smoke test passed at ${BASE_URL}"
