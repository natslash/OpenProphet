#!/bin/bash

echo "Stopping OpenProphet servers..."

# Kill Node.js frontend process
pkill -f "node agent/server.js" 2>/dev/null
if [ $? -eq 0 ]; then
    echo "Frontend server stopped."
fi

# Kill Go backend process
pkill -f "cmd/bot/main.go" 2>/dev/null
if [ $? -eq 0 ]; then
    echo "Backend server stopped."
fi

echo "All OpenProphet servers are offline."
