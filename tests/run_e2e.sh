#!/bin/bash
set -e

# Directories
PROJECT_ROOT=$(pwd)
SERVER_DIR="$PROJECT_ROOT/server"
TEST_DIR="$PROJECT_ROOT/tests"
TEST_SERVER_DIR="$TEST_DIR/test_server"

# Cleanup any running processes on exit
cleanup() {
    echo "Cleaning up processes..."
    kill $MACHDOWN_PID 2>/dev/null || true
    kill $TEST_SERVER_PID 2>/dev/null || true
}
trap cleanup EXIT

# 1. Kill any existing processes on our ports
echo "Killing existing servers on ports 8888 and 9999..."
fuser -k 8888/tcp 2>/dev/null || true
fuser -k 9999/tcp 2>/dev/null || true
sleep 1

# 2. Build and start test server
echo "Starting test server..."
cd "$TEST_SERVER_DIR"
go build -o test_server main.go
./test_server &
TEST_SERVER_PID=$!
sleep 2 # wait to start

# 3. Build and start MachDown server
echo "Starting MachDown server..."
cd "$SERVER_DIR"
rm -rf data downloads # Clean previous state
go build -o machdown-server main.go
./machdown-server &
MACHDOWN_PID=$!
sleep 2 # wait to start

# Extract API Key from DB
API_KEY=$(sqlite3 data/machdown.db "SELECT api_key FROM server_configs LIMIT 1;")
echo "API Key: $API_KEY"

# 3. Enqueue download
echo "Enqueueing download..."
curl -s -X POST http://localhost:8888/api/downloads \
     -H "Content-Type: application/json" \
     -H "X-API-Key: $API_KEY" \
     -d '{
         "url": "http://localhost:9999/1gb.bin",
         "target_clients": ["tester"]
     }'

echo "Download enqueued. Tailing logs for 15 seconds to see chunk progress..."
sleep 15
