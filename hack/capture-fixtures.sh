#!/usr/bin/env bash
# capture-fixtures.sh — fetches read-only responses from a real UniFi
# controller and writes them to internal/unifi/testdata/ so the unit tests can
# replay them via httptest.
#
# All requests are GET. No records are created, modified, or deleted.
#
# Required env:
#   UNIFI_HOST     full base URL, e.g. https://192.168.1.1
#   UNIFI_API_KEY  api key from your controller's Integrations page
#
# Optional env:
#   UNIFI_SITE                site id (default: "default")
#   UNIFI_EXTERNAL_CONTROLLER set to 1 if you point at an external controller
#                             (changes the path prefix from /proxy/network/v2 to /v2)
#   UNIFI_SKIP_TLS_VERIFY     set to 1 to pass curl -k for self-signed certs
#
# After it runs, REVIEW each captured file before sharing — they will contain
# your real hostnames, IPs and record IDs.

set -euo pipefail

: "${UNIFI_HOST:?UNIFI_HOST must be set, e.g. https://192.168.1.1}"
: "${UNIFI_API_KEY:?UNIFI_API_KEY must be set}"

SITE="${UNIFI_SITE:-default}"
OUT_DIR="$(cd "$(dirname "$0")/.." && pwd)/internal/unifi/testdata"
mkdir -p "$OUT_DIR"

if [[ "${UNIFI_EXTERNAL_CONTROLLER:-0}" == "1" ]]; then
  RECORDS_PATH="${UNIFI_HOST}/v2/api/site/${SITE}/static-dns/"
else
  RECORDS_PATH="${UNIFI_HOST}/proxy/network/v2/api/site/${SITE}/static-dns/"
fi

CURL_OPTS=(--silent --show-error --header "X-Api-Key: ${UNIFI_API_KEY}" --header "Accept: application/json")
if [[ "${UNIFI_SKIP_TLS_VERIFY:-0}" == "1" ]]; then
  CURL_OPTS+=(--insecure)
fi

capture() {
  local name="$1" url="$2" expect_status="$3"
  local tmp_body
  tmp_body=$(mktemp)
  local meta_file="${OUT_DIR}/${name}.meta.txt"
  local http_status content_type ext

  echo ">> ${name}: GET ${url}"
  http_status=$(curl "${CURL_OPTS[@]}" -o "${tmp_body}" -w "%{http_code}\n%{content_type}" "${url}" | head -1)
  # Re-run to capture content type without clobbering the body
  content_type=$(curl "${CURL_OPTS[@]}" -o /dev/null -s -w "%{content_type}" "${url}")

  case "${content_type}" in
    application/json*) ext="json" ;;
    text/html*)        ext="html" ;;
    *)                 ext="txt"  ;;
  esac

  local body_file="${OUT_DIR}/${name}.${ext}"
  mv "${tmp_body}" "${body_file}"

  printf 'status:       %s\ncontent_type: %s\nurl:          %s\n' \
    "${http_status}" "${content_type}" "${url}" >"${meta_file}"

  if [[ "${http_status}" != "${expect_status}" ]]; then
    echo "   warning: expected HTTP ${expect_status}, got ${http_status}" >&2
  fi
  echo "   wrote ${body_file} (${http_status}, ${content_type}) and ${meta_file}"
}

# 1. The records-list endpoint. This is the hot path the client hits on every
#    Records() / ApplyChanges() call.
capture "records_list" "${RECORDS_PATH}" 200

# 2. A nonexistent record id — captures the real shape of an error response so
#    handleErrorResponse() can be tested against it.
capture "records_not_found" "${RECORDS_PATH}deadbeef-0000-0000-0000-000000000000" 404

echo
echo "Done. Review and sanitize before sharing:"
echo "   * Replace real hostnames with example.com names"
echo "   * Replace real IPs with TEST-NET addresses (192.0.2.x / 198.51.100.x / 203.0.113.x)"
echo "   * Replace record IDs (the '_id' field) with synthetic values"
echo "   * Remove any other PII"
echo
echo "Files written to: ${OUT_DIR}"
