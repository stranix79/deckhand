#!/usr/bin/env bash
# Throwaway PostgreSQL for hub development, in ./.pg on port 5499.
#   scripts/dev-pg.sh start | stop | psql
set -euo pipefail
export PATH="/opt/homebrew/opt/postgresql@16/bin:$PATH"
dir="$(cd "$(dirname "$0")/.." && pwd)/.pg"
port=5499
case "${1:-start}" in
  start)
    if [ ! -d "$dir" ]; then
      initdb -D "$dir" -U postgres --auth=trust -E UTF8 >/dev/null
      echo "port = $port" >> "$dir/postgresql.conf"
      echo "listen_addresses = '127.0.0.1'" >> "$dir/postgresql.conf"
    fi
    pg_ctl -D "$dir" -l "$dir/log" status >/dev/null 2>&1 || pg_ctl -D "$dir" -l "$dir/log" start >/dev/null
    for i in $(seq 1 20); do pg_isready -q -h 127.0.0.1 -p $port && break; sleep 0.3; done
    psql -h 127.0.0.1 -p $port -U postgres -tc "SELECT 1 FROM pg_database WHERE datname='deckhand'" | grep -q 1 || \
      psql -h 127.0.0.1 -p $port -U postgres -c "CREATE DATABASE deckhand" >/dev/null
    echo "postgres ready on 127.0.0.1:$port (db deckhand)";;
  stop) pg_ctl -D "$dir" stop >/dev/null && echo stopped;;
  psql) exec psql -h 127.0.0.1 -p $port -U postgres deckhand;;
esac
