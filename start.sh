#!/bin/bash

MODE=${1:-paper}

echo "Starting OpenProphet in $MODE mode..."

if [ "$MODE" = "live" ]; then
    echo "LIVE MODE — Bot will connect to port 4001"
    echo "NOTE: Requires ADMIN_TOKEN for double-confirmation."
    export IBKR_PORT=4001
    export ALLOW_LIVE_PORT=true
    export REQUIRE_DOUBLE_CONFIRM=true
else
    echo "PAPER MODE — Bot will connect to port 4002"
    export IBKR_PORT=4002
fi

# Build if needed
if [ ! -f ./prophet_bot ] || [ ./cmd/bot/main.go -nt ./prophet_bot ]; then
    echo "Building..."
    go build -o prophet_bot ./cmd/bot || exit 1
fi

echo ""
echo "=========================================================="
echo "OpenProphet running in $MODE mode"
echo "Dashboard:    http://localhost:4534/"
echo "Press Ctrl+C to stop."
echo "=========================================================="

./prophet_bot
