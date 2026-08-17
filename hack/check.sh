#!/bin/bash
set -e

echo "> Check"

echo "Executing golangci-lint"
which golangci-lint
go tool golangci-lint run "${SOURCE_TREES[@]}" --timeout=10m0s

echo "Check for license headers"
go tool addlicense -check \
    -ignore ".git/**" -ignore "hack/**" -ignore "**/*.yaml" \
    -ignore "**/*.yml" -ignore "resources/demo-config-files/**" -ignore "**/*.proto" \
    -ignore "website/**" \
    -ignore "flake.nix" .

echo "All checks successful"
