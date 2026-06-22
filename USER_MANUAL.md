# OpenProphet User Manual

Welcome to the OpenProphet User Manual! This living document will gradually expand to cover all operational aspects of the platform.

## 1. Starting and Stopping the Servers

OpenProphet consists of two primary components that must run concurrently:
1. **The Backend (Go)**: Handles trading logic, order execution, AI intent generation, and strict risk guardrails.
2. **The Frontend (Node.js)**: Serves the Web UI dashboard and proxies requests to the backend.

### 1.1 Prerequisites & Configuration
Before starting the servers, ensure that your Interactive Brokers (IBKR) Gateway or TWS workstation is running and logged in.
- **Paper Trading**: Typically listens on port `4002`.
- **Live Trading**: Typically listens on port `4001`.

**Security Configuration (`.env.backend`)**
To authorize live trades and critical actions, you must create a `.env.backend` file in the root directory. This file is **only** read by the Go backend to ensure the Web Dashboard cannot autonomously authorize its own trades.

Create a `.env.backend` file and add your secure token:
```bash
ADMIN_TOKEN="your_super_secret_password_here"
```
*Note: Do not place `ADMIN_TOKEN` in your regular `.env` file, and never export it directly in your shell.*

### 1.2 Starting the Servers

**Starting the Backend (Go)**
Open a terminal in the root directory of the project and run:
```bash
# This will start the backend (defaults to Paper mode on port 4002)
go run ./cmd/bot/main.go
```
> **Note**: To start the backend in **Live Mode** (port 4001), strict double-confirm guardrails are enforced. You must provide specific environment variables or the bot will fatally crash on startup:
> ```bash
> IBKR_PORT=4001 ALLOW_LIVE_PORT=true REQUIRE_DOUBLE_CONFIRM=true go run ./cmd/bot/main.go
> ```

**Starting the Frontend Web UI (Node.js)**
Open a *second* terminal in the root directory and start the Node.js server:
```bash
node agent/server.js
```
Once both are running, you can access your dashboard by opening a web browser and navigating to:
`http://localhost:4534/dashboard/` (or whichever port your Node server bound to).

### 1.3 Stopping the Servers

To gracefully stop the servers:
1. Go to the terminal running the **Node.js Frontend** and press `Ctrl + C`.
2. Go to the terminal running the **Go Backend** and press `Ctrl + C`.

If the processes are running in the background or you need to force kill them, you can run the following command from any terminal:
```bash
pkill -f "node agent/server.js"
pkill -f "cmd/bot/main.go"
```
