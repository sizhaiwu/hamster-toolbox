#!/bin/sh
set -eu
cd "$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)"
version=$(tr -d '\r\n' < VERSION)
mkdir -p work/build
go build -buildvcs=false -trimpath -ldflags "-s -w -X main.appVersion=$version-dev" -o work/build/hamster-toolbox .
printf '%s\n' 'Built: work/build/hamster-toolbox' 'Start: ./work/build/hamster-toolbox'
