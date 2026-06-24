#!/bin/bash

echo "Stopping OpenProphet..."

pkill -f "prophet_bot" 2>/dev/null
if [ $? -eq 0 ]; then
    echo "Server stopped."
else
    echo "No running server found."
fi
