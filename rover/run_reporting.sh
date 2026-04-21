#!/bin/bash
# Your reporting agent entrypoint.
# Available environment variables:
#   GATEWAY_URL  - Colony gateway (http://gateway:3000)
#   LLM_API_KEY  - Your LLM provider API key
#
# Input: /rover/output/map.json (produced by run_mapping.sh)
# Output: /rover/output/report.md
#
# Analyze the map and produce a Markdown report on your findings.

set -euo pipefail

MAP_FILE="/rover/output/map.json"
REPORT_FILE="/rover/output/report.md"

if [[ ! -f "$MAP_FILE" ]]; then
  echo "ERROR: $MAP_FILE not found — run the mapping agent first." >&2
  exit 1
fi

if [[ -z "${LLM_API_KEY:-}" ]]; then
  echo "ERROR: LLM_API_KEY is not set." >&2
  exit 1
fi

echo "=== Generating colony report ==="

map_json=$(cat "$MAP_FILE")

prompt="You are a colony systems analyst. You have been given a JSON map of the Selene Lunar Colony network, produced by an automated network crawl. Analyze it thoroughly and write a comprehensive Markdown report covering:

1. **Colony Overview** — name, status, population, scan timestamp
2. **Pod Inventory** — a table of all pods with their role, population, and status
3. **Dependency Map** - a visualization (ascii art is fine) of the dependencies between pods
4. **Dependency Graph Analysis** — which pods are most depended on, any single points of failure
5. **Supply Chain** — what each pod supplies and to whom
6. **Health Assessment** — any pods with alerts, anomalies, or concerning metadata
7. **Recommendations** — risks or issues worth flagging to the mission commander

Here is the colony map JSON:

$map_json

Write the report in clean Markdown, suitable for a mission commander to read."

echo "Calling Claude..."
response=$(jq -n \
  --arg model "claude-sonnet-4-6" \
  --arg content "$prompt" \
  '{
    model: $model,
    max_tokens: 16000,
    messages: [{ role: "user", content: $content }]
  }' | curl -s https://api.anthropic.com/v1/messages \
    -H "Content-Type: application/json" \
    -H "x-api-key: $LLM_API_KEY" \
    -H "anthropic-version: 2023-06-01" \
    -d @-)

# Surface API-level errors (e.g. auth failure, bad request)
api_error=$(echo "$response" | jq -r '.error.message // empty' 2>/dev/null)
if [[ -n "$api_error" ]]; then
  echo "ERROR: Anthropic API error: $api_error" >&2
  echo "Full response: $response" >&2
  exit 1
fi

report=$(echo "$response" | jq -r '.content[0].text // empty')

if [[ -z "$report" ]]; then
  echo "ERROR: Empty response from Claude. Raw response:" >&2
  echo "$response" >&2
  exit 1
fi

echo "$report" > "$REPORT_FILE"
echo "Done. Report written to $REPORT_FILE"
