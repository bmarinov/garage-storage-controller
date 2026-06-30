#!/usr/bin/env bash
# Wrapper for govulncheck. Allows excluding specific vulnerabilities.
# Issue at https://github.com/golang/go/issues/59507
#
# Usage: govulncheck.sh [exclude-file]
#   exclude-file: file with vuln IDs to exclude, one per line (default: known-vulns.txt next to this script)
set -Eeuo pipefail

script_dir="$(cd "$(dirname "$0")" && pwd)"
repo_root="$(cd "$script_dir/.." && pwd)"
exclude_file="${1:-$script_dir/known-vulns.txt}"
exclude_file="$(cd "$(dirname "$exclude_file")" && pwd)/$(basename "$exclude_file")"

cd "$repo_root"

# Build JSON array of excluded vuln IDs from the text file (strips comments and blank lines).
excludeVulns="$(grep -v '^#' "$exclude_file" | grep -v '^[[:space:]]*$' | jq -Rnc '[inputs]')"
export excludeVulns

json="$(govulncheck -json ./... || true)"

vulns="$(jq <<<"$json" -cs '
  (map(.osv // empty | {key: .id, value: .}) | from_entries) as $meta
  | map(.finding // empty | select((.trace[0].function // "") != "") | .osv)
  | unique
  | map($meta[.])
')"

filtered="$(jq <<<"$vulns" -c '
  (env.excludeVulns | fromjson) as $exclude
  | map(select(.id as $id | $exclude | index($id) | not))
')"

text="$(jq <<<"$filtered" -r 'map("- \(.id) (aka \(.aliases | join(", ")))\n\n\t\(.details | gsub("\n"; "\n\t"))") | join("\n\n")')"

if [ -z "$text" ]; then
  echo "govulncheck passed (all findings are in the known-excluded list)"
  exit 0
else
  printf '%s\n' "$text"
  exit 1
fi
