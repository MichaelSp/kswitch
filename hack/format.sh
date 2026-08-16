#!/bin/bash
set -e

echo "> Format"

go tool goimports -l -w $@

addlicense -c "The Kswitch authors" pkg/
addlicense -c "The Kswitch authors" cmd/
addlicense -c "The Kswitch authors" types/
