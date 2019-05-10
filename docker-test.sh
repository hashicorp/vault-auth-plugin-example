#!/usr/bin/env bash

set -e

GOOS=linux go build

$(dirname $0)/docker-test-nobuild.sh

