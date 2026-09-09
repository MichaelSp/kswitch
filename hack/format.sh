#!/bin/bash
set -e

echo "> Format"

go tool goimports -l -w "$@"

go tool addlicense -c "The Kswitch authors" pkg/
go tool addlicense -c "The Kswitch authors" cmd/
go tool addlicense -c "The Kswitch authors" types/
