#!/usr/bin/env bash
set -euo pipefail
umask 077

script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
cd -- "$script_dir"

domain="${1:-}"
if [[ ! "$domain" =~ ^[a-zA-Z0-9][a-zA-Z0-9.-]*\.[a-zA-Z0-9-]+$ ]]; then
  printf 'Usage: ./init.sh proxy.example.com\n' >&2
  exit 1
fi

for path in .env data/config/config.yaml data/credentials.txt; do
  if [[ -e "$path" || -L "$path" ]]; then
    printf 'Existing file: %s. Setup stopped to preserve your settings.\n' "$path" >&2
    exit 1
  fi
done

management_key="$(openssl rand -hex 32)"
api_key="$(openssl rand -hex 32)"
proxy_commit="$(git -C .. rev-parse HEAD)"

mkdir -p data/config data/auths data/logs
set -o noclobber
sed -e "s/REPLACE_MANAGEMENT_KEY/$management_key/" \
    -e "s/REPLACE_API_KEY/$api_key/" \
    config.example.yaml > data/config/config.yaml
printf 'DOMAIN=%s\nPROXY_COMMIT=%s\n' "$domain" "$proxy_commit" > .env
printf 'Management key: %s\nAPI key: %s\n' "$management_key" "$api_key" > data/credentials.txt

printf 'Setup files created. Keys are in deploy/data/credentials.txt.\n'
printf 'Start with: docker compose up -d --build\n'
