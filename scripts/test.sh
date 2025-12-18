#!/bin/bash
set -e

coverage_profile="coverage.out"
html_report="coverage.html"

# Cover all packages except e2e & k8s api 
mapfile -t packages < <(
    go list ./... \
        | grep -v /e2e \
        | grep -v /api/v1alpha1
)
coverpkg=$(IFS=, ; echo "${packages[*]}")

go test "${packages[@]}" -coverpkg="$coverpkg" -coverprofile="$coverage_profile" -v
go tool cover -func="$coverage_profile"

if [[ "$1" == "--html" ]]; then
    go tool cover -html="$coverage_profile" -o "$html_report"
    echo "HTML report saved as $html_report"
fi

