#!/bin/bash

MODE=${1:-paper}

echo "Starting OpenProphet in $MODE mode..."

if [ "$MODE" = "live" ]; then
    echo "Starting Dashboard (LIVE MODE) - Bot will connect to port 4001..."
    echo "NOTE: Requires .env.backend with ADMIN_TOKEN for double-confirmation."
    export IBKR_PORT=4001
    export ALLOW_LIVE_PORT=true
    export REQUIRE_DOUBLE_CONFIRM=true
else
    echo "Starting Dashboard (PAPER MODE) - Bot will connect to port 4002..."
    export IBKR_PORT=4002
fi

echo ""
echo "=========================================================="
echo "OpenProphet is running in $MODE mode!"
echo "Dashboard:    http://localhost:4534/dashboard/"
echo "Press Ctrl+C to gracefully stop the server."
echo "=========================================================="

# Start the Node frontend (which automatically manages the Go backend)
node agent/server.js
