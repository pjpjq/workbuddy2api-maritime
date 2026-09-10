#!/bin/bash
set -euo pipefail

# Ensure persistence directories exist in /data
mkdir -p "${WB2A_AUTH_DIR:-/data/auths}"
mkdir -p "$(dirname "${WB2A_STATE_FILE:-/data/state.json}")"

# Copy default config if none exists in /data
if [ ! -f /data/config.json ] && [ -f /app/config.example.json ]; then
    cp /app/config.example.json /data/config.json
    sed -i 's|"api_key": "your-api-key-here"|"api_key": ""|g' /data/config.json
    sed -i 's|"auth_dir": "./auths"|"auth_dir": "/data/auths"|g' /data/config.json
    sed -i 's|"state_file": "./data/state.json"|"state_file": "/data/state.json"|g' /data/config.json
fi

PORT="${PORT:-18789}"
export WB2A_LISTEN="${WB2A_LISTEN:-0.0.0.0:${PORT}}"
export WB2A_AUTH_DIR="${WB2A_AUTH_DIR:-/data/auths}"
export WB2A_STATE_FILE="${WB2A_STATE_FILE:-/data/state.json}"

echo "[entrypoint] Starting workbuddy2api on ${WB2A_LISTEN}..."
echo "[entrypoint] Auth directory: ${WB2A_AUTH_DIR}"
echo "[entrypoint] State file:     ${WB2A_STATE_FILE}"

CONFIG_FILE="/data/config.json"
if [ ! -f "$CONFIG_FILE" ]; then
    CONFIG_FILE="/app/config.example.json"
fi

exec /app/wb2api -config "$CONFIG_FILE"
