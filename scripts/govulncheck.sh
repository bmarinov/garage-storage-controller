#!/usr/bin/env bash
# Wrapper for govulncheck. Allows excluding specific vulnerabilities.
# Issue at https://github.com/golang/go/issues/59507
set -Eeuo pipefail

# Vulnerabilities excluded until upstream ships a fix.
# Each entry must link to a tracking issue.
excludeVulns="$(jq -nc '[
  # docker (testcontainers dependency), no fix
  # Issue: https://github.com/bmarinov/garage-storage-controller/issues/72
  "GO-2026-4887", # CVE-2026-34040 AuthZ plugin bypass
  "GO-2026-4883", # CVE-2026-33997 plugin privilege off-by-one

  empty
]')"
export excludeVulns

# nothing to filter if govulncheck passes:
if govulncheck ./...; then
  exit 0
fi

json="$(govulncheck -json ./...)"

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
