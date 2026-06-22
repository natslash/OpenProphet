#!/bin/bash

MODE=${1:-paper}

echo "Starting OpenProphet in $MODE mode..."

# Start the Go backend
if [ "$MODE" = "live" ]; then
    echo "Starting Backend (LIVE MODE) on port 4001..."
    echo "NOTE: Requires .env.backend with ADMIN_TOKEN for double-confirmation."
    IBKR_PORT=4001 ALLOW_LIVE_PORT=true REQUIRE_DOUBLE_CONFIRM=true go run ./cmd/bot/main.go &
    GO_PID=$!
else
    echo "Starting Backend (PAPER MODE) on port 4002..."
    go run ./cmd/bot/main.go &
    GO_PID=$!
fi

# Give backend a second to initialize
sleep 2

# Start the Node frontend
echo "Starting Node.js Frontend Dashboard..."
node agent/server.js &
NODE_PID=$!

echo ""
echo "=========================================================="
echo "OpenProphet is running in $MODE mode!"
echo "Backend PID:  $GO_PID"
echo "Frontend PID: $NODE_PID"
echo "Dashboard:    http://localhost:4534/dashboard/"
echo "Press Ctrl+C to gracefully stop both servers."
echo "=========================================================="

# Trap SIGINT and SIGTERM to kill both background jobs
trap "echo -e '\nStopping OpenProphet servers...'; kill $GO_PID $NODE_PID 2>/dev/null; exit 0" SIGINT SIGTERM

# Wait for both background processes
wait $GO_PID $NODE_PID
