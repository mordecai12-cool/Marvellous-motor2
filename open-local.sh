#!/usr/bin/env bash
set -euo pipefail
cd -- "$(dirname -- "$0")"
./start-local.sh
xdg-open http://localhost:8080
