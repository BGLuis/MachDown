#!/bin/bash
set -e

PROJECT_ROOT=$(pwd)
SERVER_DIR="$PROJECT_ROOT/server"
TEST_DIR="$PROJECT_ROOT/tests"
TEST_SERVER_DIR="$TEST_DIR/test_server"

cleanup() {
    echo "Cleaning up processes..."
    fuser -k 8888/tcp 2>/dev/null || true
    fuser -k 9999/tcp 2>/dev/null || true
}
trap cleanup EXIT

cleanup

echo "Starting test server..."
cd "$TEST_SERVER_DIR"
go build -o test_server main.go
./test_server &
sleep 2

echo "Starting MachDown server..."
cd "$SERVER_DIR"
rm -rf data downloads # Clean previous state
go build -o machdown-server main.go
./machdown-server &
sleep 2

API_KEY=$(sqlite3 data/machdown.db "SELECT api_key FROM server_configs LIMIT 1;")
echo "API Key: $API_KEY"

echo "Enqueueing sequential download..."
curl -s -X POST http://localhost:8888/api/downloads \
     -H "Content-Type: application/json" \
     -H "X-API-Key: $API_KEY" \
     -d '{
         "url": "http://localhost:9999/stream",
         "target_clients": ["tester"]
     }'

echo "Tailing logs for 5 seconds..."
sleep 5
