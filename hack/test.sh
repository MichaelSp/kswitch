#!/bin/bash

set -e

echo "> Test"

GO111MODULE=on go test -race $@ | grep -v 'no test files'
