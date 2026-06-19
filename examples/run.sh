#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"
cd ..

make build

printf '\n== Marcadores detectados ==\n'
./bin/pdfsignmark --list-markers examples/marker-test.pdf

printf '\n== Dry run ==\n'
./bin/pdfsignmark --dry-run --v examples/marker-test.pdf
