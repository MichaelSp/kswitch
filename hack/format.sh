#!/bin/bash
set -e

echo "> Format"

goimports -l -w $@

addlicense -c "The Kswitch authors" pkg/
addlicense -c "The Kswitch authors" cmd/
addlicense -c "The Kswitch authors" hooks/
addlicense -c "The Kswitch authors" types/
