#!/usr/bin/env bash
set -euo pipefail
cd -- "$(dirname -- "$0")"
if command -v systemctl >/dev/null && systemctl --user is-active --quiet marvellous-motors-preview.service; then
  systemctl --user stop marvellous-motors-preview.service
  echo 'Stopped the local Marvellous Motors server.'
  exit 0
fi
if [[ ! -f .local/server.pid ]]; then echo 'No local server PID found.'; exit 0; fi
server_pid="$(cat .local/server.pid)"
server_exe="$(readlink -f "/proc/$server_pid/exe" || true)"
if [[ "$server_exe" == "$PWD/.local/marvellous-motors" ]]; then
  kill "$server_pid"
  echo 'Stopped the local Marvellous Motors server.'
else
  echo 'The saved PID is no longer this project server.'
fi
