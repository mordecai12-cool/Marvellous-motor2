#!/usr/bin/env bash
set -euo pipefail
cd -- "$(dirname -- "$0")"
mkdir -p .local
if curl -fsS http://127.0.0.1:8080/api/vehicles > /dev/null 2>&1; then
  echo 'Marvellous Motors is already running at http://localhost:8080'
  exit 0
fi
go build -o .local/marvellous-motors .
if command -v systemd-run >/dev/null && systemctl --user is-system-running >/dev/null 2>&1; then
  systemctl --user reset-failed marvellous-motors-preview.service 2>/dev/null || true
  systemd-run --user --collect --unit=marvellous-motors-preview --working-directory="$PWD" --setenv="MM_DB_PATH=${MM_DB_PATH:-.local/preview.db}" --setenv="MM_DEMO_MODE=${MM_DEMO_MODE:-true}" --setenv="MM_HOST=${MM_HOST:-127.0.0.1}" "$PWD/.local/marvellous-motors"
else
  MM_DB_PATH="${MM_DB_PATH:-.local/preview.db}" MM_DEMO_MODE="${MM_DEMO_MODE:-true}" nohup ./.local/marvellous-motors > .local/server.log 2>&1 &
  echo $! > .local/server.pid
fi
for attempt in {1..30}; do
  if curl -fsS http://127.0.0.1:8080/ > /dev/null 2>&1; then
    echo 'Marvellous Motors is running at http://localhost:8080'
    exit 0
  fi
  sleep 1
done
cat .local/server.log
exit 1
